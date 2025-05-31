#!/usr/bin/env python3
"""
Production-ready training pipeline with MLOps features
"""

import os
import json
import logging
import argparse
import pickle
from datetime import datetime, timedelta
from pathlib import Path
from typing import Dict, List, Tuple, Any, Optional
import numpy as np
import pandas as pd
from sklearn.model_selection import train_test_split, cross_val_score, TimeSeriesSplit
from sklearn.preprocessing import StandardScaler
from sklearn.metrics import (
    classification_report, roc_auc_score, f1_score, 
    precision_recall_curve, roc_curve, confusion_matrix
)
from sklearn.ensemble import RandomForestClassifier
from xgboost import XGBClassifier
import lightgbm as lgb
import optuna
from skl2onnx import to_onnx
import mlflow
import mlflow.sklearn
import matplotlib.pyplot as plt
import seaborn as sns
from model_management import ModelRegistry
from skl2onnx.common.data_types import FloatTensorType  # NEW

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


class ProductionTrainer:
    """Production-ready model training with MLOps features"""
    
    def __init__(self, experiment_name: str = "bitunix-trading-bot"):
        self.experiment_name = experiment_name
        self.mlflow_tracking_uri = os.getenv("MLFLOW_TRACKING_URI", "file:./mlruns")
        mlflow.set_tracking_uri(self.mlflow_tracking_uri)
        mlflow.set_experiment(experiment_name)
        
    def load_and_validate_data(self, data_path: str) -> pd.DataFrame:
        """Load and validate training data"""
        logger.info(f"Loading data from {data_path}")
        
        # Load data
        with open(data_path, 'r') as f:
            data = json.load(f)
        
        df = pd.DataFrame(data)
        
        # Validate required columns
        required_cols = ['timestamp', 'tick_ratio', 'depth_ratio', 'price_dist', 'label'
        ]
        missing = set(required_cols) - set(df.columns)
        if missing:
            raise ValueError(f"Missing required columns: {missing}")
        
        # Data quality checks
        logger.info(f"Data shape: {df.shape}")
        logger.info(f"Label distribution:\n{df['label'].value_counts()}")
        
        # Check for missing values
        missing_pct = df.isnull().sum() / len(df) * 100
        if (missing_pct > 0).any():
            logger.warning(f"Missing values:\n{missing_pct[missing_pct > 0]}")
        
        # Check for data anomalies
        for col in ['tick_ratio', 'depth_ratio', 'price_dist']:
            q1, q99 = df[col].quantile([0.01, 0.99])
            outliers = ((df[col] < q1) | (df[col] > q99)).sum()
            if outliers > 0:
                logger.warning(f"{col}: {outliers} outliers ({outliers/len(df)*100:.2f}%)")
        
        # Sort by timestamp for time series split
        df['timestamp'] = pd.to_datetime(df['timestamp'])
        df = df.sort_values('timestamp').reset_index(drop=True)
        
        return df
    
    def create_features(self, df: pd.DataFrame) -> Tuple[np.ndarray, np.ndarray, List[str]]:
        """Create features with engineering"""
        # Basic features
        feature_cols = ['tick_ratio', 'depth_ratio', 'price_dist']
        
        # Feature engineering
        # Add rolling statistics
        for col in feature_cols:
            df[f'{col}_roll_mean_10'] = df[col].rolling(10, min_periods=1).mean()
            df[f'{col}_roll_std_10'] = df[col].rolling(10, min_periods=1).std().fillna(0)
        
        # Add interaction features
        df['tick_depth_interaction'] = df['tick_ratio'] * df['depth_ratio']
        df['price_volume_interaction'] = df['price_dist'] * df['depth_ratio']
        
        # Get all feature columns
        all_features = [col for col in df.columns if col not in ['timestamp', 'label']]
        
        X = df[all_features].values
        y = df['label'].values
        
        logger.info(f"Feature shape: {X.shape}")
        logger.info(f"Features: {all_features}")
        
        return X, y, all_features
    
    def optimize_hyperparameters(self, X_train: np.ndarray, y_train: np.ndarray, 
                               n_trials: int = 50) -> Dict[str, Any]:
        """Optimize hyperparameters using Optuna"""
        logger.info("Starting hyperparameter optimization...")
        
        def objective(trial):
            # Model selection
            model_name = trial.suggest_categorical('model', ['xgboost', 'lightgbm', 'random_forest'])
            
            if model_name == 'xgboost':
                params = {
                    'n_estimators': trial.suggest_int('n_estimators', 50, 300),
                    'max_depth': trial.suggest_int('max_depth', 3, 10),
                    'learning_rate': trial.suggest_float('learning_rate', 0.01, 0.3, log=True),
                    'subsample': trial.suggest_float('subsample', 0.6, 1.0),
                    'colsample_bytree': trial.suggest_float('colsample_bytree', 0.6, 1.0),
                    'reg_alpha': trial.suggest_float('reg_alpha', 0.0, 1.0),
                    'reg_lambda': trial.suggest_float('reg_lambda', 0.0, 1.0),
                }
                model = XGBClassifier(**params, use_label_encoder=False, eval_metric='logloss')
                
            elif model_name == 'lightgbm':
                params = {
                    'n_estimators': trial.suggest_int('n_estimators', 50, 300),
                    'max_depth': trial.suggest_int('max_depth', 3, 10),
                    'learning_rate': trial.suggest_float('learning_rate', 0.01, 0.3, log=True),
                    'subsample': trial.suggest_float('subsample', 0.6, 1.0),
                    'colsample_bytree': trial.suggest_float('colsample_bytree', 0.6, 1.0),
                    'reg_alpha': trial.suggest_float('reg_alpha', 0.0, 1.0),
                    'reg_lambda': trial.suggest_float('reg_lambda', 0.0, 1.0),
                }
                model = lgb.LGBMClassifier(**params, verbosity=-1)
                
            else:  # random_forest
                params = {
                    'n_estimators': trial.suggest_int('n_estimators', 50, 300),
                    'max_depth': trial.suggest_int('max_depth', 3, 20),
                    'min_samples_split': trial.suggest_int('min_samples_split', 2, 20),
                    'min_samples_leaf': trial.suggest_int('min_samples_leaf', 1, 10),
                }
                model = RandomForestClassifier(**params, n_jobs=-1)
            
            # Time series cross-validation
            tscv = TimeSeriesSplit(n_splits=5)
            scores = cross_val_score(model, X_train, y_train, cv=tscv, scoring='roc_auc')
            
            return scores.mean()
        
        # Create study
        study = optuna.create_study(direction='maximize')
        study.optimize(objective, n_trials=n_trials)
        
        logger.info(f"Best score: {study.best_value:.4f}")
        logger.info(f"Best params: {study.best_params}")
        
        return study.best_params
    
    def train_final_model(self, X_train: np.ndarray, y_train: np.ndarray, 
                         params: Dict[str, Any]) -> Any:
        """Train final model with best parameters"""
        model_name = params.pop('model')
        
        if model_name == 'xgboost':
            model = XGBClassifier(**params, use_label_encoder=False, eval_metric='logloss')
        elif model_name == 'lightgbm':
            model = lgb.LGBMClassifier(**params, verbosity=-1)
        else:
            model = RandomForestClassifier(**params, n_jobs=-1)
        
        # Train model
        model.fit(X_train, y_train)
        
        return model, model_name
    
    def evaluate_model(self, model: Any, X_test: np.ndarray, y_test: np.ndarray, 
                      feature_names: List[str]) -> Dict[str, Any]:
        """Comprehensive model evaluation"""
        # Predictions
        y_pred = model.predict(X_test)
        y_prob = model.predict_proba(X_test)[:, 1]
        
        # Metrics
        metrics = {
            'accuracy': (y_pred == y_test).mean(),
            'roc_auc': roc_auc_score(y_test, y_prob),
            'f1_score': f1_score(y_test, y_pred),
            'classification_report': classification_report(y_test, y_pred, output_dict=True),
        }
        
        # Feature importance
        if hasattr(model, 'feature_importances_'):
            importance = pd.DataFrame({
                'feature': feature_names,
                'importance': model.feature_importances_
            }).sort_values('importance', ascending=False)
            metrics['feature_importance'] = importance.to_dict('records')
        
        # Confusion matrix
        cm = confusion_matrix(y_test, y_pred)
        metrics['confusion_matrix'] = cm.tolist()
        
        # Plot metrics
        self._plot_evaluation_metrics(y_test, y_prob, cm)
        
        return metrics
    
    def _plot_evaluation_metrics(self, y_true: np.ndarray, y_prob: np.ndarray, 
                                cm: np.ndarray):
        """Plot evaluation metrics"""
        fig, axes = plt.subplots(2, 2, figsize=(12, 10))
        
        # ROC curve
        fpr, tpr, _ = roc_curve(y_true, y_prob)
        axes[0, 0].plot(fpr, tpr)
        axes[0, 0].plot([0, 1], [0, 1], 'k--')
        axes[0, 0].set_xlabel('False Positive Rate')
        axes[0, 0].set_ylabel('True Positive Rate')
        axes[0, 0].set_title('ROC Curve')
        
        # Precision-Recall curve
        precision, recall, _ = precision_recall_curve(y_true, y_prob)
        axes[0, 1].plot(recall, precision)
        axes[0, 1].set_xlabel('Recall')
        axes[0, 1].set_ylabel('Precision')
        axes[0, 1].set_title('Precision-Recall Curve')
        
        # Confusion matrix
        sns.heatmap(cm, annot=True, fmt='d', ax=axes[1, 0])
        axes[1, 0].set_xlabel('Predicted')
        axes[1, 0].set_ylabel('Actual')
        axes[1, 0].set_title('Confusion Matrix')
        
        # Probability distribution
        axes[1, 1].hist(y_prob[y_true == 0], alpha=0.5, label='Class 0', bins=30)
        axes[1, 1].hist(y_prob[y_true == 1], alpha=0.5, label='Class 1', bins=30)
        axes[1, 1].set_xlabel('Predicted Probability')
        axes[1, 1].set_ylabel('Count')
        axes[1, 1].set_title('Probability Distribution')
        axes[1, 1].legend()
        
        plt.tight_layout()
        plt.savefig('evaluation_metrics.png')
        mlflow.log_artifact('evaluation_metrics.png')
    
    def convert_to_onnx(self, model: Any, X_sample: np.ndarray, 
                       output_path: str) -> str:
        """Convert sklearn model to ONNX"""
        logger.info("Converting model to ONNX format...")

        initial_type = [('float_input', FloatTensorType([None, X_sample.shape[1]]))]  # FIX
        onnx_model = to_onnx(model, initial_types=initial_type,
                             target_opset=12, options={'zipmap': False})
        
        # Save model
        with open(output_path, 'wb') as f:
            f.write(onnx_model.SerializeToString())
        
        logger.info(f"Model saved to {output_path}")
        return output_path
    
    def run_training_pipeline(self, data_path: str, output_dir: str = "./models",
                            optimize_hyperparams: bool = True, n_trials: int = 50):
        """Run complete training pipeline"""
        with mlflow.start_run():
            # Load data
            df = self.load_and_validate_data(data_path)
            mlflow.log_param("data_rows", len(df))
            mlflow.log_param("data_path", data_path)
            
            # Create features
            X, y, feature_names = self.create_features(df)
            mlflow.log_param("num_features", X.shape[1])
            mlflow.log_param("features", feature_names)
            
            # Train-test split (time-based)
            split_idx = int(len(X) * 0.8)
            X_train, X_test = X[:split_idx], X[split_idx:]
            y_train, y_test = y[:split_idx], y[split_idx:]
            
            logger.info(f"Train shape: {X_train.shape}, Test shape: {X_test.shape}")
            
            # Scale features
            scaler = StandardScaler()
            X_train_scaled = scaler.fit_transform(X_train)
            X_test_scaled = scaler.transform(X_test)
            
            # Optimize hyperparameters
            if optimize_hyperparams:
                best_params = self.optimize_hyperparameters(X_train_scaled, y_train, n_trials)
            else:
                best_params = {
                    'model': 'xgboost',
                    'n_estimators': 100,
                    'max_depth': 5,
                    'learning_rate': 0.1,
                    'subsample': 0.8,
                    'colsample_bytree': 0.8,
                    'reg_alpha': 0.1,
                    'reg_lambda': 0.1,
                }
            
            mlflow.log_params(best_params)
            
            # Train final model
            model, model_name = self.train_final_model(X_train_scaled, y_train, best_params)
            
            # Evaluate model
            metrics = self.evaluate_model(model, X_test_scaled, y_test, feature_names)
            
            # Log metrics
            mlflow.log_metric("accuracy", metrics['accuracy'])
            mlflow.log_metric("roc_auc", metrics['roc_auc'])
            mlflow.log_metric("f1_score", metrics['f1_score'])
            
            logger.info(f"Test Accuracy: {metrics['accuracy']:.4f}")
            logger.info(f"Test ROC-AUC: {metrics['roc_auc']:.4f}")
            logger.info(f"Test F1-Score: {metrics['f1_score']:.4f}")
            
            # Save artifacts
            timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
            
            # Save scaler
            scaler_path = f"{output_dir}/scaler_{timestamp}.pkl"
            with open(scaler_path, 'wb') as f:
                pickle.dump(scaler, f)
            mlflow.log_artifact(scaler_path)
            
            # Save sklearn model
            mlflow.sklearn.log_model(model, "sklearn_model")
            
            # Convert to ONNX
            onnx_path = f"{output_dir}/model_{timestamp}.onnx"
            self.convert_to_onnx(model, X_train_scaled, onnx_path)
            mlflow.log_artifact(onnx_path)
            
            # Save metadata
            metadata = {
                'version': f"v{timestamp}",
                'trained_at': datetime.now().isoformat(),
                'model_type': model_name,
                'features': feature_names,
                'accuracy': metrics['accuracy'],
                'validation_accuracy': metrics['roc_auc'],
                'training_rows': len(X_train),
                'test_rows': len(X_test),
                'metrics': metrics,
                'hyperparameters': best_params,
            }
            
            metadata_path = f"{output_dir}/model_metadata_{timestamp}.json"
            with open(metadata_path, 'w') as f:
                json.dump(metadata, f, indent=2)
            mlflow.log_artifact(metadata_path)

            # ALSO save/copy as stable name for serving code
            latest_path = f"{output_dir}/model_metadata.json"
            with open(latest_path, 'w') as f:
                json.dump(metadata, f, indent=2)
            mlflow.log_artifact(latest_path)

            # Register model in model registry
            registry = ModelRegistry(output_dir)
            version = registry.register_model(onnx_path, metadata)
            
            logger.info(f"Training completed. Model version: {version.version}")
            
            return onnx_path, metrics, metadata


def main():
    parser = argparse.ArgumentParser(description="Production model training")
    parser.add_argument('--data-file', required=True, help='Training data file')
    parser.add_argument('--output-dir', default='./models', help='Output directory')
    parser.add_argument('--no-optimize', action='store_true', help='Skip hyperparameter optimization')
    parser.add_argument('--n-trials', type=int, default=50, help='Number of optimization trials')
    parser.add_argument('--experiment-name', default='bitunix-trading-bot', help='MLflow experiment name')
    
    args = parser.parse_args()
    
    # Create output directory
    os.makedirs(args.output_dir, exist_ok=True)
    
    # Run training
    trainer = ProductionTrainer(args.experiment_name)
    model_path, metrics, metadata = trainer.run_training_pipeline(
        args.data_file,
        args.output_dir,
        optimize_hyperparams=not args.no_optimize,
        n_trials=args.n_trials
    )
    
    print(f"\nTraining completed successfully!")
    print(f"Model saved to: {model_path}")
    print(f"Accuracy: {metrics['accuracy']:.4f}")
    print(f"ROC-AUC: {metrics['roc_auc']:.4f}")


if __name__ == "__main__":
    main()
