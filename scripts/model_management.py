#!/usr/bin/env python3
"""
Model management utilities for production deployment
"""

import os
import json
import hashlib
import shutil
import argparse
import logging
from datetime import datetime
from pathlib import Path
from typing import Dict, Any, Optional, List
import onnx
import onnxruntime as ort
import numpy as np
from dataclasses import dataclass, asdict

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


@dataclass
class ModelVersion:
    """Model version information"""
    version: str
    timestamp: datetime
    sha256: str
    size_bytes: int
    accuracy: float
    validation_accuracy: float
    training_rows: int
    features: List[str]
    input_shape: List[int]
    output_shape: List[int]
    metadata: Dict[str, Any]


class ModelRegistry:
    """Manages model versions and deployments"""
    
    def __init__(self, storage_path: str, s3_bucket: Optional[str] = None):
        self.storage_path = Path(storage_path)
        self.storage_path.mkdir(parents=True, exist_ok=True)
        self.registry_file = self.storage_path / "registry.json"
        self.s3_bucket = s3_bucket
        self.registry = self._load_registry()
    
    def _load_registry(self) -> Dict[str, Any]:
        """Load existing registry or create new one"""
        if self.registry_file.exists():
            with open(self.registry_file, 'r') as f:
                return json.load(f)
        return {"versions": {}, "current": None, "deployments": {}}
    
    def _save_registry(self):
        """Save registry to disk"""
        with open(self.registry_file, 'w') as f:
            json.dump(self.registry, f, indent=2, default=str)
    
    def register_model(self, model_path: str, metadata: Dict[str, Any]) -> ModelVersion:
        """Register a new model version"""
        model_path = Path(model_path)
        
        # Validate model
        self._validate_model(model_path)
        
        # Calculate model hash
        sha256 = self._calculate_sha256(model_path)
        
        # Generate version
        version = datetime.now().strftime("v%Y%m%d_%H%M%S")
        
        # Get model info
        model_info = self._get_model_info(model_path)
        
        # Create version object
        model_version = ModelVersion(
            version=version,
            timestamp=datetime.now(),
            sha256=sha256,
            size_bytes=model_path.stat().st_size,
            accuracy=metadata.get('accuracy', 0.0),
            validation_accuracy=metadata.get('validation_accuracy', 0.0),
            training_rows=metadata.get('training_rows', 0),
            features=metadata.get('features', ['tick_ratio', 'depth_ratio', 'price_dist']),
            input_shape=model_info['input_shape'],
            output_shape=model_info['output_shape'],
            metadata=metadata
        )
        
        # Store model
        stored_path = self._store_model(model_path, version)
        
        # Update registry
        self.registry['versions'][version] = asdict(model_version)
        self.registry['versions'][version]['path'] = str(stored_path)
        self._save_registry()
        
        # Upload to S3 if configured
        if self.s3_bucket:
            self._upload_to_s3(stored_path, version)
        
        logger.info(f"Registered model version {version}")
        return model_version
    
    def _validate_model(self, model_path: Path):
        """Validate ONNX model"""
        try:
            # Load and check model
            model = onnx.load(str(model_path))
            onnx.checker.check_model(model)
            
            # Test inference
            session = ort.InferenceSession(str(model_path))
            input_shape = session.get_inputs()[0].shape
            
            # Create test input
            test_input = np.random.randn(1, 3).astype(np.float32)
            
            # Run inference
            outputs = session.run(None, {session.get_inputs()[0].name: test_input})
            
            if len(outputs) == 0 or outputs[0].shape[1] != 2:
                raise ValueError("Model output shape is incorrect")
                
        except Exception as e:
            raise ValueError(f"Model validation failed: {e}")
    
    def _calculate_sha256(self, file_path: Path) -> str:
        """Calculate SHA256 hash of file"""
        sha256_hash = hashlib.sha256()
        with open(file_path, "rb") as f:
            for byte_block in iter(lambda: f.read(4096), b""):
                sha256_hash.update(byte_block)
        return sha256_hash.hexdigest()
    
    def _get_model_info(self, model_path: Path) -> Dict[str, Any]:
        """Get model input/output shapes"""
        session = ort.InferenceSession(str(model_path))
        return {
            'input_shape': list(session.get_inputs()[0].shape),
            'output_shape': list(session.get_outputs()[0].shape),
            'input_name': session.get_inputs()[0].name,
            'output_name': session.get_outputs()[0].name,
        }
    
    def _store_model(self, model_path: Path, version: str) -> Path:
        """Copy model into the versioned storage folder and return new path."""
        dest_dir = self.storage_path / version
        dest_dir.mkdir(parents=True, exist_ok=True)
        dst = dest_dir / model_path.name
        shutil.copy2(model_path, dst)
        return dst

    def _upload_to_s3(self, model_path: Path, version: str):
        """Dummy S3 uploader (no-op if bucket not configured)."""
        if not self.s3_bucket:
            return
        # Lazy import to avoid hard dependency when S3 is unused
        import boto3, botocore
        s3 = boto3.client("s3")
        try:
            s3.upload_file(str(model_path), self.s3_bucket, f"{version}/{model_path.name}")
        except botocore.exceptions.BotoCoreError as e:
            logger.warning(f"S3 upload failed: {e}")

    def deploy_model(self, version: str, environment: str = "production"):
        """Mark a version as deployed for a given environment."""
        if version not in self.registry["versions"]:
            raise ValueError(f"Unknown version {version}")
        self.registry["deployments"][environment] = {
            "version": version,
            "timestamp": datetime.utcnow().isoformat()
        }
        # Track current production version for quick access
        if environment == "production":
            self.registry["current"] = version
        self._save_registry()
        logger.info(f"Deployed version {version} to {environment}")
    
    def get_current_version(self) -> Optional[str]:
        """Get current production version"""
        return self.registry.get('current')
    
    def get_model_path(self, version: Optional[str] = None) -> Optional[Path]:
        """Return local path of a model version (current if None)."""
        if version is None:
            version = self.registry.get("current")
        if version and version in self.registry["versions"]:
            return Path(self.registry["versions"][version]["path"])
        return None
    
    def list_versions(self) -> List[Dict[str, Any]]:
        """Return sorted list of versions with meta information."""
        versions: List[Dict[str, Any]] = []
        for v, info in self.registry["versions"].items():
            versions.append(
                {
                    "version": v,
                    "timestamp": info.get("timestamp"),
                    "accuracy": info.get("accuracy"),
                    "size_mb": info.get("size_mb"),
                    "is_current": v == self.registry.get("current"),
                    "deployments": [
                        env for env, d in self.registry["deployments"].items() if d["version"] == v
                    ],
                }
            )
        return sorted(versions, key=lambda x: x["timestamp"], reverse=True)
    
    def rollback(self, environment: str = "production"):
        """Rollback to previous version"""
        current = self.registry['deployments'].get(environment, {}).get('version')
        if not current:
            raise ValueError(f"No current deployment in {environment}")
        
        # Find previous version
        versions = sorted(self.registry['versions'].keys(), reverse=True)
        current_idx = versions.index(current)
        
        if current_idx < len(versions) - 1:
            previous = versions[current_idx + 1]
            self.deploy_model(previous, environment)
            logger.info(f"Rolled back {environment} from {current} to {previous}")
        else:
            raise ValueError("No previous version available for rollback")
    
    def cleanup_old_versions(self, keep_count: int = 5):
        """Remove old model versions, keeping the most recent ones"""
        versions = sorted(self.registry['versions'].items(), 
                         key=lambda x: x[1]['timestamp'], 
                         reverse=True)
        
        # Determine versions to keep
        keep_versions = set()
        
        # Keep recent versions
        for version, _ in versions[:keep_count]:
            keep_versions.add(version)
        
        # Keep deployed versions
        for deploy in self.registry['deployments'].values():
            keep_versions.add(deploy['version'])
        
        # Keep current version
        if self.registry.get('current'):
            keep_versions.add(self.registry['current'])
        
        # Remove old versions
        for version, info in versions:
            if version not in keep_versions:
                version_dir = self.storage_path / version
                if version_dir.exists():
                    shutil.rmtree(version_dir)
                del self.registry['versions'][version]
                logger.info(f"Removed old version {version}")
        
        self._save_registry()


def main():
    parser = argparse.ArgumentParser(description="Model management CLI")
    subparsers = parser.add_subparsers(dest='command', help='Commands')
    
    # Register command
    register_parser = subparsers.add_parser('register', help='Register new model')
    register_parser.add_argument('model_path', help='Path to ONNX model')
    register_parser.add_argument('--accuracy', type=float, required=True)
    register_parser.add_argument('--validation-accuracy', type=float, required=True)
    register_parser.add_argument('--training-rows', type=int, required=True)
    register_parser.add_argument('--metadata', type=json.loads, default={})
    
    # Deploy command
    deploy_parser = subparsers.add_parser('deploy', help='Deploy model version')
    deploy_parser.add_argument('version', help='Model version to deploy')
    deploy_parser.add_argument('--environment', default='production')
    
    # List command
    subparsers.add_parser('list', help='List all versions')
    
    # Rollback command
    rollback_parser = subparsers.add_parser('rollback', help='Rollback deployment')
    rollback_parser.add_argument('--environment', default='production')
    
    # Cleanup command
    cleanup_parser = subparsers.add_parser('cleanup', help='Clean up old versions')
    cleanup_parser.add_argument('--keep', type=int, default=5)
    
    # Global arguments
    parser.add_argument('--storage-path', default='./models', help='Model storage path')
    parser.add_argument('--s3-bucket', help='S3 bucket for model storage')
    
    args = parser.parse_args()
    
    # Initialize registry
    registry = ModelRegistry(args.storage_path, args.s3_bucket)
    
    if args.command == 'register':
        version = registry.register_model(args.model_path)
        print(f"Model registered with version: {version}")
        
    elif args.command == 'deploy':
        success = registry.deploy_model(args.version, args.environment)
        if success:
            print(f"Model {args.version} deployed to {args.environment}")
        else:
            print(f"Failed to deploy model {args.version}")
            return 1
            
    elif args.command == 'list':
        versions = registry.list_versions()
        for v in versions:
            current = " (current)" if v['is_current'] else ""
            deployments = f" [deployed to: {', '.join(v['deployments'])}]" if v['deployments'] else ""
            print(f"{v['version']} - {v['timestamp']} - Accuracy: {v['accuracy']:.4f} - Size: {v['size_mb']:.2f}MB{current}{deployments}")
            
    elif args.command == 'rollback':
        registry.rollback(args.environment)
        
    elif args.command == 'cleanup':
        registry.cleanup_old_versions(args.keep)
        print(f"Cleaned up old versions, keeping {args.keep} most recent")


if __name__ == "__main__":
    exit(main())
