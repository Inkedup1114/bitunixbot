import numpy as np
import onnxruntime as ort
import threading
import queue
import time
from typing import List, Tuple, Optional

class FastPredictor:
    def __init__(self, model_path: str, batch_size: int = 32):
        # Initialize ONNX Runtime session with optimized settings
        sess_options = ort.SessionOptions()
        sess_options.graph_optimization_level = ort.GraphOptimizationLevel.ORT_ENABLE_ALL
        sess_options.intra_op_num_threads = 1  # Single thread for inference
        sess_options.inter_op_num_threads = 1  # No parallel execution
        
        # Load model with optimized execution provider
        self.session = ort.InferenceSession(
            model_path,
            sess_options,
            providers=['CPUExecutionProvider']
        )
        
        self.batch_size = batch_size
        self.input_name = self.session.get_inputs()[0].name
        self.output_name = self.session.get_outputs()[0].name
        
        # Pre-allocate buffers for batch processing
        self.input_buffer = np.zeros((batch_size, 3), dtype=np.float32)
        self.output_buffer = np.zeros((batch_size, 2), dtype=np.float32)
        
        # Thread-safe queue for batch processing
        self.queue = queue.Queue()
        self.result_queue = queue.Queue()
        self.running = True
        
        # Start worker thread
        self.worker = threading.Thread(target=self._worker_loop)
        self.worker.daemon = True
        self.worker.start()
    
    def _worker_loop(self):
        while self.running:
            try:
                # Get batch of features
                batch = []
                batch_ids = []
                
                # Get first item with timeout
                try:
                    features, batch_id = self.queue.get(timeout=0.001)
                    batch.append(features)
                    batch_ids.append(batch_id)
                except queue.Empty:
                    continue
                
                # Try to get more items up to batch_size
                while len(batch) < self.batch_size:
                    try:
                        features, batch_id = self.queue.get_nowait()
                        batch.append(features)
                        batch_ids.append(batch_id)
                    except queue.Empty:
                        break
                
                # Process batch
                if batch:
                    self._process_batch(batch, batch_ids)
            
            except Exception as e:
                print(f"Error in worker loop: {e}")
                time.sleep(0.001)  # Prevent tight loop on error
    
    def _process_batch(self, batch: List[np.ndarray], batch_ids: List[int]):
        # Convert batch to numpy array
        batch_size = len(batch)
        self.input_buffer[:batch_size] = np.array(batch)
        
        # Run inference
        outputs = self.session.run(
            [self.output_name],
            {self.input_name: self.input_buffer[:batch_size]}
        )[0]
        
        # Store results
        for i, batch_id in enumerate(batch_ids):
            self.result_queue.put((batch_id, outputs[i]))
    
    def predict(self, features: np.ndarray, timeout: float = 0.005) -> Optional[np.ndarray]:
        if not self.running:
            return None
        
        # Generate unique batch ID
        batch_id = id(features)
        
        # Add to queue
        self.queue.put((features, batch_id))
        
        # Wait for result with timeout
        start_time = time.time()
        while time.time() - start_time < timeout:
            try:
                result_id, result = self.result_queue.get_nowait()
                if result_id == batch_id:
                    return result
                else:
                    # Put back other results
                    self.result_queue.put((result_id, result))
            except queue.Empty:
                time.sleep(0.0001)  # Small sleep to prevent tight loop
        
        return None
    
    def shutdown(self):
        self.running = False
        if self.worker.is_alive():
            self.worker.join(timeout=1.0)

if __name__ == "__main__":
    # Test the predictor
    predictor = FastPredictor("model.onnx")
    
    # Test single prediction
    features = np.array([0.1, 0.2, 0.3], dtype=np.float32)
    result = predictor.predict(features)
    print(f"Single prediction result: {result}")
    
    # Test batch prediction
    features_batch = [np.array([0.1, 0.2, 0.3], dtype=np.float32) for _ in range(10)]
    results = [predictor.predict(f) for f in features_batch]
    print(f"Batch prediction results: {results}")
    
    predictor.shutdown() 