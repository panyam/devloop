#!/usr/bin/env python3
"""
Model Training Script for Python Data Science Example

This script demonstrates a typical machine learning training workflow
that would be monitored and auto-restarted by devloop.
"""

import argparse
import os
import yaml
import pandas as pd
import numpy as np
from sklearn.ensemble import RandomForestClassifier
from sklearn.model_selection import train_test_split
from sklearn.metrics import accuracy_score, classification_report
import joblib
import time
from datetime import datetime

def load_config(config_path):
    """Load configuration from YAML file."""
    with open(config_path, 'r') as f:
        return yaml.safe_load(f)

def load_data(data_path):
    """Load dataset from CSV file."""
    print(f"Loading data from {data_path}")
    try:
        df = pd.read_csv(data_path)
        print(f"Data loaded successfully: {df.shape}")
        return df
    except FileNotFoundError:
        print("Data file not found. Generating synthetic data...")
        return generate_synthetic_data()

def generate_synthetic_data():
    """Generate synthetic dataset for demonstration."""
    np.random.seed(42)
    n_samples = 1000
    n_features = 10
    
    # Generate random features
    X = np.random.randn(n_samples, n_features)
    
    # Generate target with some pattern
    y = (X[:, 0] + X[:, 1] + np.random.randn(n_samples) * 0.1 > 0).astype(int)
    
    # Create DataFrame
    feature_names = [f'feature_{i}' for i in range(n_features)]
    df = pd.DataFrame(X, columns=feature_names)
    df['target'] = y
    
    print(f"Generated synthetic data: {df.shape}")
    return df

def preprocess_data(df, config):
    """Preprocess the data according to configuration."""
    print("Preprocessing data...")
    
    # Basic preprocessing
    df = df.dropna()
    
    # Feature selection (if specified in config)
    feature_cols = [col for col in df.columns if col != 'target']
    X = df[feature_cols]
    y = df['target']
    
    print(f"Features: {X.shape}, Target: {y.shape}")
    return X, y

def train_model(X, y, config):
    """Train the model."""
    print("Training model...")
    
    # Split data
    test_size = config.get('test_size', 0.2)
    random_state = config.get('random_state', 42)
    
    X_train, X_test, y_train, y_test = train_test_split(
        X, y, test_size=test_size, random_state=random_state
    )
    
    # Model parameters from config
    model_params = config.get('model_params', {})
    model = RandomForestClassifier(**model_params)
    
    # Training with progress simulation
    print("Training in progress...")
    start_time = time.time()
    model.fit(X_train, y_train)
    training_time = time.time() - start_time
    
    # Evaluate
    y_pred = model.predict(X_test)
    accuracy = accuracy_score(y_test, y_pred)
    
    print(f"Training completed in {training_time:.2f} seconds")
    print(f"Test Accuracy: {accuracy:.4f}")
    print("\nClassification Report:")
    print(classification_report(y_test, y_pred))
    
    return model, {
        'accuracy': accuracy,
        'training_time': training_time,
        'model_params': model_params,
        'timestamp': datetime.now().isoformat()
    }

def save_model(model, metrics, config):
    """Save the trained model and metrics."""
    model_dir = config.get('model_dir', 'models')
    os.makedirs(model_dir, exist_ok=True)
    
    # Save model
    model_path = os.path.join(model_dir, 'model.pkl')
    joblib.dump(model, model_path)
    print(f"Model saved to {model_path}")
    
    # Save metrics
    metrics_path = os.path.join(model_dir, 'metrics.yaml')
    with open(metrics_path, 'w') as f:
        yaml.dump(metrics, f)
    print(f"Metrics saved to {metrics_path}")

def main():
    parser = argparse.ArgumentParser(description='Train ML model')
    parser.add_argument('--config', required=True, help='Path to config file')
    args = parser.parse_args()
    
    print(f"Starting training run at {datetime.now()}")
    print(f"Config file: {args.config}")
    
    # Load configuration
    config = load_config(args.config)
    
    # Load data
    data_path = config.get('data_path', 'data/dataset.csv')
    df = load_data(data_path)
    
    # Preprocess
    X, y = preprocess_data(df, config)
    
    # Train
    model, metrics = train_model(X, y, config)
    
    # Save
    save_model(model, metrics, config)
    
    print("Training completed successfully!")

if __name__ == "__main__":
    main()