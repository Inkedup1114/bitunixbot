#!/usr/bin/env python3
"""
Model Validation and Performance Regression Testing
Tests ML model performance against baseline metrics and validates model integrity.
"""

import argparse
import json
import logging
import os
import sys
import time
from datetime import datetime, timedelta
from pathlib import Path
from typing import Dict, List, Tuple, Optional, Any

import numpy as np
import onnxruntime as ort
import pandas as pd
from sklearn.metrics import (
    accuracy_score, precision_score, recall_score, f1_score,
    roc_auc_score, confusion_matrix, classification_report
)
from sklearn.model_selection import cross_val_score
from sklearn.preprocessing import StandardScaler
import joblib

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler('model_validation.log'),
        logging.StreamHandler(sys.stdout)
    ]
)
logger = logging.getLogger(__name__)

class ModelValidator:
    """Comprehensive model validation and regression testing."""
    
    def __init__(self, model_path: str, test_data_path: str, baseline_metrics_path: str = None):
        self.model_path = Path(model_path)
        self.test_data_path = Path(test_data_path)
        self.baseline_metrics_path = Path(baseline_metrics_path) if baseline_metrics_path else None
        
        # Validation thresholds
        self.min_accuracy = 0.55
        self.min_precision = 0.50
        self.min_recall = 0.50
        self.min_f1_score = 0.50
        self.min_auc = 0.55
        
        # Regression thresholds (% degradation allowed)
        self.max_accuracy_degradation = 0.05
        self.max_precision_degradation = 0.05
        self.max_recall_degradation = 0.05
        self.max_f1_degradation = 0.05
        self.max_auc_degradation = 0.05
        
        self.session = None
        self.scaler = None
        
    def load_model(self) -> bool:
        """Load and validate ONNX model."""
        try:
            if not self.model_path.exists():
                logger.error(f"Model file not found: {self.model_path}")
                return False
            
            # Configure ONNX Runtime session
            providers = ['CPUExecutionProvider']
            session_options = ort.SessionOptions()
            session_options.graph_optimization_level = ort.GraphOptimizationLevel.ORT_ENABLE_ALL
            
            self.session = ort.InferenceSession(
                str(self.model_path), 
                sess_options=session_options,
                providers=providers
            )
            
            logger.info(f"Model loaded successfully from {self.model_path}")
            
            # Log model metadata
            input_info = self.session.get_inputs()[0]
            output_info = self.session.get_outputs()[0]
            logger.info(f"Model input shape: {input_info.shape}, type: {input_info.type}")
            logger.info(f"Model output shape: {output_info.shape}, type: {output_info.type}")
            
            return True
            
        except Exception as e:
            logger.error(f"Failed to load model: {e}")
            return False
    
    def load_scaler(self, scaler_path: str = None) -> bool:
        """Load feature scaler."""
        try:
            if scaler_path:
                scaler_file = Path(scaler_path)
            else:
                # Default scaler location
                scaler_file = self.model_path.parent / "scaler.joblib"
            
            if not scaler_file.exists():
                logger.warning(f"Scaler file not found: {scaler_file}")
                return False
            
            self.scaler = joblib.load(scaler_file)
            logger.info(f"Scaler loaded from {scaler_file}")
            return True
            
        except Exception as e:
            logger.error(f"Failed to load scaler: {e}")
            return False
    
    def load_test_data(self) -> Tuple[Optional[np.ndarray], Optional[np.ndarray]]:
        """Load test dataset."""
        try:
            if not self.test_data_path.exists():
                logger.error(f"Test data file not found: {self.test_data_path}")
                return None, None
            
            if self.test_data_path.suffix == '.json':
                with open(self.test_data_path, 'r') as f:
                    data = json.load(f)
                
                # Extract features and labels
                features = []
                labels = []
                
                for item in data:
                    if 'features' in item and 'label' in item:
                        features.append(item['features'])
                        labels.append(item['label'])
                
                X = np.array(features, dtype=np.float32)
                y = np.array(labels, dtype=np.int32)
                
            elif self.test_data_path.suffix == '.csv':
                df = pd.read_csv(self.test_data_path)
                # Assume last column is the label
                X = df.iloc[:, :-1].values.astype(np.float32)
                y = df.iloc[:, -1].values.astype(np.int32)
            
            else:
                logger.error(f"Unsupported test data format: {self.test_data_path.suffix}")
                return None, None
            
            logger.info(f"Test data loaded: {X.shape[0]} samples, {X.shape[1]} features")
            logger.info(f"Label distribution: {np.bincount(y)}")
            
            return X, y
            
        except Exception as e:
            logger.error(f"Failed to load test data: {e}")
            return None, None
    
    def preprocess_features(self, X: np.ndarray) -> np.ndarray:
        """Preprocess features using loaded scaler."""
        try:
            if self.scaler is not None:
                X_scaled = self.scaler.transform(X)
                logger.info("Features scaled using loaded scaler")
                return X_scaled
            else:
                logger.warning("No scaler available, using raw features")
                return X
                
        except Exception as e:
            logger.error(f"Feature preprocessing failed: {e}")
            return X
    
    def predict_batch(self, X: np.ndarray) -> Tuple[np.ndarray, np.ndarray]:
        """Make predictions on batch of samples."""
        try:
            if self.session is None:
                raise ValueError("Model not loaded")
            
            # Get input name
            input_name = self.session.get_inputs()[0].name
            
            # Make predictions
            outputs = self.session.run(None, {input_name: X})
            
            # Extract probabilities and predictions
            if len(outputs) >= 2:
                # Model outputs both probabilities and predictions
                probabilities = outputs[0]
                predictions = outputs[1]
            else:
                # Model outputs only probabilities
                probabilities = outputs[0]
                predictions = (probabilities[:, 1] > 0.5).astype(np.int32)
            
            return probabilities, predictions
            
        except Exception as e:
            logger.error(f"Batch prediction failed: {e}")
            return None, None
    
    def calculate_metrics(self, y_true: np.ndarray, y_pred: np.ndarray, 
                         y_prob: np.ndarray = None) -> Dict[str, float]:
        """Calculate comprehensive performance metrics."""
        try:
            metrics = {}
            
            # Basic classification metrics
            metrics['accuracy'] = accuracy_score(y_true, y_pred)
            metrics['precision'] = precision_score(y_true, y_pred, average='weighted', zero_division=0)
            metrics['recall'] = recall_score(y_true, y_pred, average='weighted', zero_division=0)
            metrics['f1_score'] = f1_score(y_true, y_pred, average='weighted', zero_division=0)
            
            # AUC if probabilities available
            if y_prob is not None and y_prob.shape[1] >= 2:
                try:
                    metrics['auc'] = roc_auc_score(y_true, y_prob[:, 1])
                except Exception as e:
                    logger.warning(f"AUC calculation failed: {e}")
                    metrics['auc'] = 0.0
            else:
                metrics['auc'] = 0.0
            
            # Confusion matrix
            cm = confusion_matrix(y_true, y_pred)
            metrics['confusion_matrix'] = cm.tolist()
            
            # Additional metrics
            if len(np.unique(y_true)) == 2:  # Binary classification
                tn, fp, fn, tp = cm.ravel()
                metrics['true_negative'] = int(tn)
                metrics['false_positive'] = int(fp)
                metrics['false_negative'] = int(fn)
                metrics['true_positive'] = int(tp)
                metrics['specificity'] = tn / (tn + fp) if (tn + fp) > 0 else 0.0
                metrics['sensitivity'] = tp / (tp + fn) if (tp + fn) > 0 else 0.0
            
            return metrics
            
        except Exception as e:
            logger.error(f"Metrics calculation failed: {e}")
            return {}
    
    def load_baseline_metrics(self) -> Dict[str, float]:
        """Load baseline metrics for regression testing."""
        try:
            if not self.baseline_metrics_path or not self.baseline_metrics_path.exists():
                logger.warning("No baseline metrics file found")
                return {}
            
            with open(self.baseline_metrics_path, 'r') as f:
                baseline = json.load(f)
            
            logger.info(f"Baseline metrics loaded from {self.baseline_metrics_path}")
            return baseline.get('metrics', {})
            
        except Exception as e:
            logger.error(f"Failed to load baseline metrics: {e}")
            return {}
    
    def save_metrics(self, metrics: Dict[str, Any], output_path: str = None):
        """Save metrics to file."""
        try:
            if output_path is None:
                output_path = f"validation_results_{datetime.now().strftime('%Y%m%d_%H%M%S')}.json"
            
            output_data = {
                'timestamp': datetime.now().isoformat(),
                'model_path': str(self.model_path),
                'test_data_path': str(self.test_data_path),
                'metrics': metrics,
                'validation_status': self.validate_metrics(metrics),
                'regression_status': self.check_regression(metrics)
            }
            
            with open(output_path, 'w') as f:
                json.dump(output_data, f, indent=2, default=str)
            
            logger.info(f"Metrics saved to {output_path}")
            
        except Exception as e:
            logger.error(f"Failed to save metrics: {e}")
    
    def validate_metrics(self, metrics: Dict[str, float]) -> Dict[str, bool]:
        """Validate metrics against minimum thresholds."""
        validation_status = {}
        
        # Check each metric against threshold
        checks = [
            ('accuracy', self.min_accuracy),
            ('precision', self.min_precision),
            ('recall', self.min_recall),
            ('f1_score', self.min_f1_score),
            ('auc', self.min_auc)
        ]
        
        for metric_name, threshold in checks:
            if metric_name in metrics:
                passed = metrics[metric_name] >= threshold
                validation_status[metric_name] = passed
                
                status = "PASS" if passed else "FAIL"
                logger.info(f"Validation {metric_name}: {metrics[metric_name]:.4f} >= {threshold:.4f} [{status}]")
            else:
                validation_status[metric_name] = False
                logger.warning(f"Metric {metric_name} not found in results")
        
        return validation_status
    
    def check_regression(self, current_metrics: Dict[str, float]) -> Dict[str, bool]:
        """Check for performance regression against baseline."""
        regression_status = {}
        baseline_metrics = self.load_baseline_metrics()
        
        if not baseline_metrics:
            logger.info("No baseline metrics available for regression testing")
            return {}
        
        # Check each metric for regression
        checks = [
            ('accuracy', self.max_accuracy_degradation),
            ('precision', self.max_precision_degradation),
            ('recall', self.max_recall_degradation),
            ('f1_score', self.max_f1_degradation),
            ('auc', self.max_auc_degradation)
        ]
        
        for metric_name, max_degradation in checks:
            if metric_name in current_metrics and metric_name in baseline_metrics:
                baseline_value = baseline_metrics[metric_name]
                current_value = current_metrics[metric_name]
                degradation = (baseline_value - current_value) / baseline_value
                
                no_regression = degradation <= max_degradation
                regression_status[metric_name] = no_regression
                
                status = "PASS" if no_regression else "REGRESSION"
                logger.info(f"Regression {metric_name}: {current_value:.4f} vs baseline {baseline_value:.4f} "
                          f"(degradation: {degradation:.3f}) [{status}]")
            else:
                regression_status[metric_name] = False
                logger.warning(f"Cannot check regression for {metric_name} - missing data")
        
        return regression_status
    
    def run_validation(self, output_path: str = None, save_results: bool = True) -> bool:
        """Run complete model validation pipeline."""
        logger.info("Starting model validation pipeline...")
        start_time = time.time()
        
        try:
            # Load model
            if not self.load_model():
                return False
            
            # Load scaler (optional)
            self.load_scaler()
            
            # Load test data
            X_test, y_test = self.load_test_data()
            if X_test is None or y_test is None:
                return False
            
            # Preprocess features
            X_test_scaled = self.preprocess_features(X_test)
            
            # Make predictions
            logger.info("Making predictions on test set...")
            y_prob, y_pred = self.predict_batch(X_test_scaled)
            if y_prob is None or y_pred is None:
                return False
            
            # Calculate metrics
            logger.info("Calculating performance metrics...")
            metrics = self.calculate_metrics(y_test, y_pred, y_prob)
            
            if not metrics:
                return False
            
            # Validate metrics
            validation_status = self.validate_metrics(metrics)
            regression_status = self.check_regression(metrics)
            
            # Add timing information
            validation_time = time.time() - start_time
            metrics['validation_time_seconds'] = validation_time
            
            # Log summary
            logger.info(f"Validation completed in {validation_time:.2f} seconds")
            logger.info(f"Performance summary:")
            for metric_name, value in metrics.items():
                if isinstance(value, (int, float)) and not metric_name.startswith('true_') and not metric_name.startswith('false_'):
                    logger.info(f"  {metric_name}: {value:.4f}")
            
            # Check overall validation status
            validation_passed = all(validation_status.values()) if validation_status else False
            regression_passed = all(regression_status.values()) if regression_status else True
            
            overall_status = validation_passed and regression_passed
            
            if overall_status:
                logger.info("✅ Model validation PASSED")
            else:
                logger.error("❌ Model validation FAILED")
                if not validation_passed:
                    logger.error("  - Minimum performance thresholds not met")
                if not regression_passed:
                    logger.error("  - Performance regression detected")
            
            # Save results
            if save_results:
                self.save_metrics(metrics, output_path)
            
            return overall_status
            
        except Exception as e:
            logger.error(f"Validation pipeline failed: {e}")
            return False

def main():
    """Main function for command-line usage."""
    parser = argparse.ArgumentParser(description="Model Validation and Regression Testing")
    parser.add_argument("--model", "-m", required=True, help="Path to ONNX model file")
    parser.add_argument("--test-data", "-d", required=True, help="Path to test dataset (JSON/CSV)")
    parser.add_argument("--baseline", "-b", help="Path to baseline metrics file")
    parser.add_argument("--scaler", "-s", help="Path to feature scaler file")
    parser.add_argument("--output", "-o", help="Output path for validation results")
    parser.add_argument("--min-accuracy", type=float, default=0.55, help="Minimum accuracy threshold")
    parser.add_argument("--min-precision", type=float, default=0.50, help="Minimum precision threshold")
    parser.add_argument("--min-recall", type=float, default=0.50, help="Minimum recall threshold")
    parser.add_argument("--min-f1", type=float, default=0.50, help="Minimum F1 score threshold")
    parser.add_argument("--min-auc", type=float, default=0.55, help="Minimum AUC threshold")
    parser.add_argument("--max-degradation", type=float, default=0.05, help="Maximum allowed degradation (0.05 = 5%)")
    
    args = parser.parse_args()
    
    # Create validator
    validator = ModelValidator(args.model, args.test_data, args.baseline)
    
    # Set custom thresholds
    validator.min_accuracy = args.min_accuracy
    validator.min_precision = args.min_precision
    validator.min_recall = args.min_recall
    validator.min_f1_score = args.min_f1
    validator.min_auc = args.min_auc
    
    # Set degradation thresholds
    validator.max_accuracy_degradation = args.max_degradation
    validator.max_precision_degradation = args.max_degradation
    validator.max_recall_degradation = args.max_degradation
    validator.max_f1_degradation = args.max_degradation
    validator.max_auc_degradation = args.max_degradation
    
    # Override scaler path if provided
    if args.scaler:
        validator.scaler_path = args.scaler
    
    # Run validation
    success = validator.run_validation(args.output)
    
    # Exit with appropriate code
    sys.exit(0 if success else 1)

if __name__ == "__main__":
    main()
