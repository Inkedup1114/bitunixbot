#!/usr/bin/env python3

import warnings
import logging
import os
import json
from typing import Tuple, Optional
from datetime import datetime, timedelta
import numpy as np
import pandas as pd

# Suppress the precision warnings from sklearn
warnings.filterwarnings('ignore', category=UserWarning, module='sklearn.metrics._classification')

"""
ML Training Pipeline for Bitunix Bot
Trains a GradientBoostingClassifier on historical features with σ-reversion labels.
"""

# ML libraries
from sklearn.ensemble import GradientBoostingClassifier
from sklearn.model_selection import train_test_split, RandomizedSearchCV
from sklearn.metrics import classification_report, roc_auc_score, f1_score
from sklearn.preprocessing import StandardScaler
from sklearn.pipeline import Pipeline
import onnxruntime as ort
from skl2onnx import convert_sklearn
from skl2onnx.common.data_types import FloatTensorType
import shutil                      # add once for reuse

# Suppress specific warnings
warnings.filterwarnings('ignore', category=UserWarning, module='sklearn')
warnings.filterwarnings('ignore', category=FutureWarning, module='sklearn')
warnings.filterwarnings('ignore', category=DeprecationWarning, module='sklearn')

# Setup logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')

# Configuration
FEATURE_COLS = ['tick_ratio', 'depth_ratio', 'price_dist']
LOOKHEAD_SECONDS = 60
SIGMA_THRESHOLD = 1.5  # σ threshold for reversal detection
MODEL_VERSION = datetime.now().strftime("%Y%m%d")

# ------------------------------------------------------------------
# Global logger (accessible from all functions)
logger = logging.getLogger(__name__)
if not logger.handlers:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s - %(levelname)s - %(message)s"
    )
# ------------------------------------------------------------------

# Alternative labeling methods
def volatility_breakout_labeling(df, threshold=2.0):
    rolling_vol = df['price'].rolling(window=20).std()
    price_change = df['price'].pct_change(periods=10)
    return abs(price_change) > threshold * rolling_vol

def momentum_labeling(df, threshold=0.02):
    returns = df['price'].pct_change(periods=20)
    return abs(returns) > threshold

def load_bolt_data(data_file: str, symbol: str = "BTCUSDT") -> Tuple[pd.DataFrame, pd.DataFrame]:
    """
    Load features and prices from BoltDB export.
    The export_data.go script should be run first to create the JSON file:
    go run scripts/export_data.go -db data/features.db -output scripts/training_data.json -symbol BTCUSDT
    """
    
    try:
        # Load unified JSON file
        if not os.path.exists(data_file):
            logging.warning(f"⚠️  Data file {data_file} not found. Creating sample data for demonstration...")
            return create_sample_data(symbol)
        
        # Load newline-delimited JSON format
        data_records = []
        with open(data_file, 'r') as f:
            for line in f:
                line = line.strip()
                if line:
                    data_records.append(json.loads(line))
        
        if len(data_records) == 0:
            logging.warning("⚠️  No data records found. Creating sample data for demonstration...")
            return create_sample_data(symbol)
        
        # Convert to DataFrame
        df = pd.DataFrame(data_records)
        
        # Convert timestamp from Unix to datetime
        df['timestamp'] = pd.to_datetime(df['timestamp'], unit='s')
        
        # Filter by symbol if needed
        if symbol:
            df = df[df['symbol'] == symbol]
        
        # Sort by timestamp
        df = df.sort_values('timestamp').reset_index(drop=True)
        
        # Split into features and prices DataFrames
        features_df = df[['symbol', 'timestamp', 'tick_ratio', 'depth_ratio', 'price_dist', 
                         'price', 'vwap', 'std_dev', 'bid_vol', 'ask_vol']].copy()
        
        prices_df = df[['symbol', 'timestamp', 'price', 'vwap', 'std_dev']].copy()
        
        logging.info(f"✓ Loaded {len(features_df)} feature records from {data_file}")
        logging.info(f"   Time range: {df['timestamp'].min()} to {df['timestamp'].max()}")
        return features_df, prices_df
        
    except (FileNotFoundError, json.JSONDecodeError, KeyError) as e:
        logging.warning(f"⚠️  Error loading data file {data_file}: {e}")
        logging.warning("   Creating sample data for demonstration...")
        return create_sample_data(symbol)

def create_sample_data(symbol: str, n_samples: int = 10000) -> Tuple[pd.DataFrame, pd.DataFrame]:
    """Create realistic sample data for demonstration"""
    np.random.seed(42)
    
    # Generate timestamps
    start_time = datetime.now() - timedelta(days=7)
    timestamps = [start_time + timedelta(seconds=i*5) for i in range(n_samples)]
    
    # Generate realistic features
    tick_ratios = np.random.normal(0, 0.3, n_samples)
    depth_ratios = np.random.normal(0, 0.2, n_samples)
    price_dists = np.random.normal(0, 1.5, n_samples)
    
    # Generate correlated price movements
    prices = 50000 + np.cumsum(np.random.normal(0, 10, n_samples))
    vwaps = prices + np.random.normal(0, 5, n_samples)
    std_devs = np.abs(np.random.normal(20, 5, n_samples))
    
    features_df = pd.DataFrame({
        'symbol': symbol,
        'timestamp': timestamps,
        'tick_ratio': tick_ratios,
        'depth_ratio': depth_ratios,
        'price_dist': price_dists,
        'price': prices,
        'vwap': vwaps,
        'std_dev': std_devs,
        'bid_vol': np.random.exponential(100, n_samples),
        'ask_vol': np.random.exponential(100, n_samples),
    })
    
    prices_df = pd.DataFrame({
        'symbol': symbol,
        'timestamp': timestamps,
        'price': prices,
        'vwap': vwaps,
        'std_dev': std_devs,
    })
    
    return features_df, prices_df

def create_labels_vectorized(features_df: pd.DataFrame, prices_df: pd.DataFrame) -> pd.DataFrame:
    """
    Create binary labels based on 60-second σ-reversion using vectorized operations.
    Label = 1 if price reverts by >1.5σ within 60 seconds.
    Since features_df and prices_df are the same data, we just need to look ahead in time.
    """
    labeled_df = features_df.copy()
    
    # Ensure timestamps are sorted
    labeled_df = labeled_df.sort_values('timestamp').reset_index(drop=True)
    
    # Calculate future timestamps for lookhead
    future_timestamps = labeled_df['timestamp'] + pd.Timedelta(seconds=LOOKHEAD_SECONDS)
    
    labels = np.zeros(len(labeled_df), dtype=int)
    
    # Process in batches for memory efficiency
    batch_size = 1000
    for batch_start in range(0, len(labeled_df), batch_size):
        batch_end = min(batch_start + batch_size, len(labeled_df))
        
        for i in range(batch_start, batch_end):
            row = labeled_df.iloc[i]
            current_time = row['timestamp']
            future_time = future_timestamps.iloc[i]
            current_price = row['price']
            current_vwap = row['vwap']
            current_std = row['std_dev']
            
            # Skip if std_dev is too small (avoid division by zero)
            if current_std <= 0.1:
                continue
            
            # Get future prices within the lookhead window
            time_mask = (labeled_df['timestamp'] > current_time) & (labeled_df['timestamp'] <= future_time)
            future_prices = labeled_df.loc[time_mask, 'price']
            
            if len(future_prices) == 0:
                continue
            
            # Calculate current price distance from VWAP in σ units
            current_dist = (current_price - current_vwap) / current_std
            
            # Check for reversion based on current position
            reversal_detected = False
            
            if abs(current_dist) > 0.5:  # Only check reversions if we're away from VWAP
                if current_dist > 0:  # Price above VWAP, look for downward reversion
                    min_price = future_prices.min()
                    min_dist = (min_price - current_vwap) / current_std
                    if current_dist - min_dist >= SIGMA_THRESHOLD:
                        reversal_detected = True
                        
                elif current_dist < 0:  # Price below VWAP, look for upward reversion
                    max_price = future_prices.max()
                    max_dist = (max_price - current_vwap) / current_std
                    if max_dist - current_dist >= SIGMA_THRESHOLD:
                        reversal_detected = True
            
            labels[i] = 1 if reversal_detected else 0
        
        # Log progress for large datasets
        if batch_end % 5000 == 0:
            current_reversal_rate = labels[:batch_end].mean()
            logging.info(f"   Processed {batch_end}/{len(labeled_df)} samples, current reversal rate: {current_reversal_rate:.3f}")
    
    labeled_df['label'] = labels
    reversal_rate = labeled_df['label'].mean()
    logging.info(f"✓ Created labels: {reversal_rate:.3f} reversal rate ({labeled_df['label'].sum()}/{len(labeled_df)})")
    
    # Log class distribution for monitoring
    if reversal_rate < 0.05:
        logging.warning(f"⚠️  Low reversal rate ({reversal_rate:.3f}). Consider adjusting SIGMA_THRESHOLD or LOOKHEAD_SECONDS")
    elif reversal_rate > 0.5:
        logging.warning(f"⚠️  High reversal rate ({reversal_rate:.3f}). Consider increasing SIGMA_THRESHOLD")
    
    return labeled_df

def create_labels(features_df: pd.DataFrame, prices_df: pd.DataFrame) -> pd.DataFrame:
    """
    Create binary labels based on 60-second σ-reversion.
    Label = 1 if price reverts by >1.5σ within 60 seconds.
    """
    return create_labels_vectorized(features_df, prices_df)

def clean_data(df: pd.DataFrame) -> pd.DataFrame:
    """Clean data by removing NaNs, outliers, and invalid entries"""
    logging.info(f"📊 Initial data shape: {df.shape}")
    
    # Remove NaN values
    df = df.dropna(subset=FEATURE_COLS + ['label'])
    logging.info(f"   After NaN removal: {df.shape}")
    
    # Remove extreme outliers (beyond 5σ)
    for col in FEATURE_COLS:
        if col in df.columns:
            mean_val = df[col].mean()
            std_val = df[col].std()
            lower_bound = mean_val - 5 * std_val
            upper_bound = mean_val + 5 * std_val
            df = df[(df[col] >= lower_bound) & (df[col] <= upper_bound)]
    
    logging.info(f"   After outlier removal: {df.shape}")
    
    # Remove rows where std_dev is too small (division by zero protection)
    df = df[df['std_dev'] > 0.1]
    logging.info(f"   After std_dev filter: {df.shape}")
    
    return df.reset_index(drop=True)

def train_model(df: pd.DataFrame) -> Tuple[GradientBoostingClassifier, StandardScaler, dict]:
    """Train GradientBoostingClassifier with hyperparameter tuning and class imbalance handling"""
    
    # Prepare features
    X = df[FEATURE_COLS].values
    y = df['label'].values
    
    # Scale features
    scaler = StandardScaler()
    X_scaled = scaler.fit_transform(X)
    
    # Check class distribution before splitting
    class_counts = np.bincount(y)
    class_ratio = class_counts[0] / class_counts[1] if class_counts[1] > 0 else float('inf')
    logging.info(f"📊 Class distribution: {dict(enumerate(class_counts))}")
    logging.info(f"   Class ratio (0:1): {class_ratio:.2f}")
    
    # Handle severe class imbalance
    if class_ratio > 20:
        logging.warning(f"⚠️  Severe class imbalance detected (ratio: {class_ratio:.2f})")
        # Use stratified sampling to ensure both classes in test set
        try:
            X_train, X_test, y_train, y_test = train_test_split(
                X_scaled, y, test_size=0.2, random_state=42, stratify=y
            )
        except ValueError:
            logging.warning("   Cannot stratify - not enough samples in minority class")
            X_train, X_test, y_train, y_test = train_test_split(
                X_scaled, y, test_size=0.2, random_state=42
            )
    else:
        X_train, X_test, y_train, y_test = train_test_split(
            X_scaled, y, test_size=0.2, random_state=42, stratify=y
        )
    
    logging.info(f"📈 Training on {len(X_train)} samples, testing on {len(X_test)} samples")
    
    # Hyperparameter tuning with RandomizedSearchCV for faster search
    param_distributions = {
        'n_estimators': [100, 200, 300, 500],
        'max_depth': [3, 5, 7, 9],
        'learning_rate': [0.01, 0.05, 0.1, 0.2],
        'subsample': [0.7, 0.8, 0.9, 1.0],
        'min_samples_split': [2, 5, 10],
    }
    
    # Use class_weight='balanced' to handle class imbalance
    gb_classifier = GradientBoostingClassifier(
        random_state=42,
        validation_fraction=0.1,
        n_iter_no_change=10,  # Early stopping
        tol=1e-4
    )
    
    # Use RandomizedSearchCV for faster hyperparameter search
    random_search = RandomizedSearchCV(
        gb_classifier, param_distributions, n_iter=50, cv=5,
        scoring='roc_auc', n_jobs=-1, random_state=42
    )
    
    logging.info("🔍 Running randomized hyperparameter search...")
    random_search.fit(X_train, y_train)
    
    best_model = random_search.best_estimator_
    logging.info(f"✓ Best parameters: {random_search.best_params_}")
    
    # Evaluate model
    y_pred = best_model.predict(X_test)
    y_pred_proba = best_model.predict_proba(X_test)[:, 1]
    
    # Calculate metrics
    auc_score = roc_auc_score(y_test, y_pred_proba) if len(np.unique(y_test)) > 1 else 0.0
    f1 = f1_score(y_test, y_pred)
    
    metrics = {
        'auc': auc_score,
        'f1': f1,
        'best_params': random_search.best_params_,
        'feature_importance': dict(zip(FEATURE_COLS, best_model.feature_importances_)),
        'cv_score': random_search.best_score_,
        'cv_results': {
            'mean_test_roc_auc': random_search.cv_results_['mean_test_roc_auc'][random_search.best_index_],
            'std_test_roc_auc': random_search.cv_results_['std_test_roc_auc'][random_search.best_index_],
            'mean_test_f1': random_search.cv_results_['mean_test_f1'][random_search.best_index_],
            'std_test_f1': random_search.cv_results_['std_test_f1'][random_search.best_index_],
            'mean_test_precision': random_search.cv_results_['mean_test_precision'][random_search.best_index_],
            'std_test_precision': random_search.cv_results_['std_test_precision'][random_search.best_index_],
            'mean_test_recall': random_search.cv_results_['mean_test_recall'][random_search.best_index_],
            'std_test_recall': random_search.cv_results_['std_test_recall'][random_search.best_index_],
        },
        'class_distribution': dict(enumerate(class_counts)),
        'class_ratio': class_ratio
    }
    
    logging.info(f"🎯 Model Performance:")
    logging.info(f"   AUC: {auc_score:.4f}")
    logging.info(f"   F1:  {f1:.4f}")
    logging.info(f"   CV AUC: {random_search.best_score_:.4f}")
    logging.info("\n📊 Classification Report:")
    logging.info(f"\n{classification_report(y_test, y_pred)}")
    
    logging.info("\n🔍 Feature Importance:")
    for feature, importance in metrics['feature_importance'].items():
        logging.info(f"   {feature}: {importance:.4f}")
    
    return best_model, scaler, metrics, X_test, y_test

def export_onnx(model, scaler, output_path, X_test, quantize=True):
    """Export model to ONNX format"""
    try:
        print("\n🔄 Converting model to ONNX format...")

        model_dir = os.path.dirname(output_path)
        if model_dir and not os.path.exists(model_dir):
            os.makedirs(model_dir)

        pipeline = Pipeline([('scaler', scaler), ('classifier', model)])
        onnx_path = output_path if output_path.endswith(".onnx") else os.path.join(model_dir, "model.onnx")

        n_features = X_test.shape[1]
        initial_type = [('float_input', FloatTensorType([None, n_features]))]

        # ----- conversion --------------------------------------------------
        onnx_model = convert_sklearn(
            pipeline,
            initial_types=initial_type,
            target_opset=11          # int, works with ORT quantiser
        )
        with open(onnx_path, "wb") as f:
            f.write(onnx_model.SerializeToString())
        print(f"✅ ONNX model saved to: {onnx_path}")

        # ----- validation --------------------------------------------------
        ort_session = ort.InferenceSession(onnx_path, providers=['CPUExecutionProvider'])
        test_input = X_test[:1].astype(np.float32)
        ort_session.run(None, {ort_session.get_inputs()[0].name: test_input})
        print("✅ ONNX model validated")

        # ----- quantisation -----------------------------------------------
        if quantize:
            try:
                from onnxruntime.quantization import quantize_dynamic, QuantType
                quantized_path = onnx_path.replace(".onnx", "_quantized.onnx")
                print("📉 Quantising model …")

                # Quantise the preprocessed model
                quantize_dynamic(
                    onnx_path,
                    quantized_path,
                    weight_type=QuantType.QInt8
                )
                print(f"✅ Quantised model generated at: {quantized_path}")

                # quick smoke-test the quantized model BEFORE overwriting the original
                print("🧪 Validating quantised model...")
                ort.InferenceSession(quantized_path).run(
                    None, {ort_session.get_inputs()[0].name: test_input}
                )
                print("✅ Quantised model validated")

                # If validation successful, replace original model
                shutil.move(quantized_path, onnx_path)
                print(f"✅ Quantised model saved to: {onnx_path}")

            except Exception as e:
                logger.warning(f"⚠️  Quantisation failed – keeping FP model ({e})")
                if os.path.exists(quantized_path):
                    os.remove(quantized_path) # Clean up the failed quantized model
                    logger.info(f"🗑️ Removed temporary quantized file: {quantized_path}")

        return True

    except Exception as e:
        logger.error(f"❌ ONNX export failed: {e}")
        import traceback; traceback.print_exc()
        return False

def test_onnx_model(model_path: str):
    """Test ONNX model inference"""
    try:
        session = ort.InferenceSession(model_path)
        
        # Test with sample input
        test_input = np.array([[0.3, -0.4, 2.1]], dtype=np.float32)
        input_name = session.get_inputs()[0].name
        result = session.run(None, {input_name: test_input})
        
        # Handle both classifier outputs (labels and probabilities)
        if len(result) >= 2:
            prediction = result[1][0]  # Get probability scores
            logging.info(f"🧪 ONNX Test: Input {test_input[0]} -> Probabilities {prediction}")
            logging.info(f"   Reversal probability: {prediction[1]:.4f}")
        else:
            prediction = result[0][0]  # Get label prediction
            logging.info(f"🧪 ONNX Test: Input {test_input[0]} -> Prediction {prediction}")
        
    except Exception as e:
        logging.error(f"❌ ONNX test failed: {e}")

def save_metrics(metrics: dict, model_path: str):
    """Save training metrics"""
    try:
        metrics_path = model_path.replace('.onnx', '_metrics.json')
        with open(metrics_path, 'w') as f:
            json.dump(metrics, f, indent=2, default=str)
        logging.info(f"✓ Saved metrics to {metrics_path}")
    except Exception as e:
        logging.error(f"❌ Failed to save metrics: {e}")

def parse_arguments():
    """Parse command line arguments"""
    parser = argparse.ArgumentParser(description='ML Training Pipeline for Bitunix Bot')
    parser.add_argument('--data-file', default='scripts/training_data.json', 
                      help='Path to JSON data file exported by export_data.go')
    parser.add_argument('--symbol', default='BTCUSDT', 
                      help='Trading symbol to train on')
    parser.add_argument('--output-dir', default='.', 
                      help='Output directory for model files')
    parser.add_argument('--lookhead-seconds', type=int, default=60,
                      help='Lookhead window in seconds for labeling')
    parser.add_argument('--sigma-threshold', type=float, default=1.5,
                      help='Sigma threshold for reversal detection')
    parser.add_argument('--no-quantize', action='store_true', 
                       help='Skip ONNX quantization')
    parser.add_argument('--min-samples', type=int, default=1000,
                      help='Minimum samples required for training')
    parser.add_argument('--verbose', action='store_true',
                      help='Enable verbose logging')
    
    return parser.parse_args()

def main():
    """Main training pipeline"""
    
    # Parse arguments
    args = parse_arguments()
    
    # Set global configuration from args
    global LOOKHEAD_SECONDS, SIGMA_THRESHOLD
    LOOKHEAD_SECONDS = args.lookhead_seconds
    SIGMA_THRESHOLD = args.sigma_threshold
    
    # Set logging level
    if args.verbose:
        logging.getLogger().setLevel(logging.DEBUG)
    
    logging.info(f"🚀 ML Training Pipeline for {args.symbol}")
    logging.info(f"   Data file: {args.data_file}")
    logging.info(f"   Output: {args.output_dir}")
    logging.info(f"   Features: {FEATURE_COLS}")
    logging.info(f"   Lookhead: {LOOKHEAD_SECONDS}s")
    logging.info(f"   σ Threshold: {SIGMA_THRESHOLD}")
    
    # Create output directory if it doesn't exist
    os.makedirs(args.output_dir, exist_ok=True)
    
    # Load data
    logging.info("\n📥 Loading data...")
    features_df, prices_df = load_bolt_data(args.data_file, args.symbol)
    
    if len(features_df) < args.min_samples:
        logging.error(f"❌ Insufficient data for training (need >{args.min_samples} samples, got {len(features_df)})")
        return
    
    # Create labels
    logging.info("\n🏷️  Creating labels...")
    labeled_df = create_labels(features_df, prices_df)
    
    # Clean data
    logging.info("\n🧹 Cleaning data...")
    clean_df = clean_data(labeled_df)
    
    if len(clean_df) < args.min_samples // 2:
        logging.error(f"❌ Insufficient clean data for training (got {len(clean_df)})")
        return
    
    # Train model
    logging.info("\n🤖 Training model...")
    result = train_model(clean_df)
    
    # Handle the return values based on what train_model actually returns
    if len(result) == 6:
        model, scaler, metrics, X_test, y_test, _ = result
    elif len(result) == 5:
        model, scaler, metrics, X_test, y_test = result
    else:
        # Fallback for the current train_model that returns 3 values
        model, scaler, metrics = result[:3]
        # We need to recreate X_test for ONNX export
        X = clean_df[FEATURE_COLS].values
        y = clean_df['label'].values
        X_scaled = scaler.fit_transform(X)
        X_train, X_test, y_train, y_test = train_test_split(
            X_scaled, y, test_size=0.2, random_state=42, stratify=y
        )
    
    # Export to ONNX
    model_path = os.path.join(args.output_dir, f"model.onnx")
    logging.info("\n📦 Exporting ONNX model...")
    
    # Ensure X_test is available for export
    if 'X_test' not in locals():
        # Create X_test if not available
        X = clean_df[FEATURE_COLS].values
        X_test = scaler.transform(X[:5])  # Just need sample for ONNX conversion
    
    export_success = export_onnx(model, scaler, model_path, X_test, quantize=not args.no_quantize)
    
    if not export_success:
        logging.error("❌ ONNX export failed!")
        return
    
    # Save metrics
    save_metrics(metrics, model_path)
    
    # Save feature info for debugging and reproducibility
    feature_info = {
        'features': FEATURE_COLS,
        'symbol': args.symbol,
        'lookhead_seconds': LOOKHEAD_SECONDS,
        'sigma_threshold': SIGMA_THRESHOLD,
        'feature_count': len(FEATURE_COLS),
        'scaler_mean': scaler.mean_.tolist() if hasattr(scaler, 'mean_') else None,
        'scaler_scale': scaler.scale_.tolist() if hasattr(scaler, 'scale_') else None,
    }
    feature_info_path = model_path.replace('.onnx', '_feature_info.json')
    with open(feature_info_path, 'w') as f:
        json.dump(feature_info, f, indent=2)
    logging.info(f"✓ Saved feature info to {feature_info_path}")
    
    # Create symlink to latest model
    latest_path = os.path.join(args.output_dir, "model.onnx")
    try:
        # Skip when model_path already equals latest_path to prevent a self-loop
        if os.path.abspath(model_path) != os.path.abspath(latest_path):
            if os.path.islink(latest_path) or os.path.exists(latest_path):
                os.remove(latest_path)
            os.symlink(os.path.basename(model_path), latest_path)
            logging.info(f"✓ Created symlink: model.onnx -> {os.path.basename(model_path)}")
    except OSError as e:
        logging.warning(f"⚠️  Could not create symlink: {e}")
    
    # Test the exported model
    test_onnx_model(model_path)
    
    logging.info(f"\n🎉 Training complete! Model ready for deployment.")
    logging.info(f"   Model saved to: {model_path}")

if __name__ == "__main__":
    main()
