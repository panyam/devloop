#!/usr/bin/env python3
"""
Test cases for the training pipeline
"""

import pytest
import tempfile
import os
import yaml
import pandas as pd
import numpy as np
from unittest.mock import patch
import sys

# Add src to path so we can import modules
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src'))

from train import load_config, generate_synthetic_data, preprocess_data, train_model

class TestTrainingPipeline:
    
    def test_load_config(self):
        """Test configuration loading."""
        config_data = {
            'model_params': {'n_estimators': 100},
            'test_size': 0.2,
            'random_state': 42
        }
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            yaml.dump(config_data, f)
            config_path = f.name
        
        try:
            loaded_config = load_config(config_path)
            assert loaded_config == config_data
        finally:
            os.unlink(config_path)
    
    def test_generate_synthetic_data(self):
        """Test synthetic data generation."""
        df = generate_synthetic_data()
        
        # Check that we get a DataFrame
        assert isinstance(df, pd.DataFrame)
        
        # Check shape
        assert df.shape[0] == 1000  # 1000 samples
        assert df.shape[1] == 11   # 10 features + 1 target
        
        # Check columns
        feature_cols = [f'feature_{i}' for i in range(10)]
        expected_cols = feature_cols + ['target']
        assert list(df.columns) == expected_cols
        
        # Check target is binary
        assert set(df['target'].unique()) <= {0, 1}
    
    def test_preprocess_data(self):
        """Test data preprocessing."""
        # Create test DataFrame
        df = pd.DataFrame({
            'feature_0': [1, 2, 3, 4, 5],
            'feature_1': [2, 3, 4, 5, 6],
            'target': [0, 1, 0, 1, 1]
        })
        
        config = {}
        X, y = preprocess_data(df, config)
        
        # Check shapes
        assert X.shape == (5, 2)  # 5 samples, 2 features
        assert y.shape == (5,)    # 5 targets
        
        # Check feature names
        assert list(X.columns) == ['feature_0', 'feature_1']
    
    def test_train_model(self):
        """Test model training."""
        # Generate test data
        np.random.seed(42)
        X = pd.DataFrame(np.random.randn(100, 3), columns=['f1', 'f2', 'f3'])
        y = pd.Series(np.random.randint(0, 2, 100))
        
        config = {
            'test_size': 0.2,
            'random_state': 42,
            'model_params': {'n_estimators': 10, 'random_state': 42}
        }
        
        model, metrics = train_model(X, y, config)
        
        # Check that we get a model
        assert model is not None
        
        # Check metrics
        assert 'accuracy' in metrics
        assert 'training_time' in metrics
        assert 'model_params' in metrics
        assert 'timestamp' in metrics
        
        # Check accuracy is reasonable (between 0 and 1)
        assert 0 <= metrics['accuracy'] <= 1
        
        # Check training time is positive
        assert metrics['training_time'] > 0

class TestDataValidation:
    
    def test_missing_data_handling(self):
        """Test handling of missing data."""
        df = pd.DataFrame({
            'feature_0': [1, 2, np.nan, 4, 5],
            'feature_1': [2, np.nan, 4, 5, 6],
            'target': [0, 1, 0, 1, 1]
        })
        
        config = {}
        X, y = preprocess_data(df, config)
        
        # Should have removed rows with NaN
        assert X.shape[0] == 3  # Only 3 complete rows
        assert y.shape[0] == 3
    
    def test_feature_types(self):
        """Test that features are numeric."""
        df = generate_synthetic_data()
        config = {}
        X, y = preprocess_data(df, config)
        
        # All features should be numeric
        for col in X.columns:
            assert pd.api.types.is_numeric_dtype(X[col])

class TestEdgeCases:
    
    def test_empty_dataframe(self):
        """Test handling of empty DataFrame."""
        df = pd.DataFrame(columns=['feature_0', 'target'])
        config = {}
        
        X, y = preprocess_data(df, config)
        
        assert len(X) == 0
        assert len(y) == 0
    
    def test_single_class_target(self):
        """Test handling of single-class target."""
        df = pd.DataFrame({
            'feature_0': [1, 2, 3, 4, 5],
            'feature_1': [2, 3, 4, 5, 6],
            'target': [1, 1, 1, 1, 1]  # All same class
        })
        
        config = {
            'test_size': 0.2,
            'random_state': 42,
            'model_params': {'n_estimators': 10, 'random_state': 42}
        }
        
        X, y = preprocess_data(df, config)
        
        # Should still work but might have warnings
        model, metrics = train_model(X, y, config)
        assert model is not None
        assert 'accuracy' in metrics

if __name__ == "__main__":
    pytest.main([__file__, "-v"])