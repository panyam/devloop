#!/usr/bin/env python3
"""
Data Preprocessing Pipeline

This script handles data preprocessing tasks that trigger when raw data changes.
"""

import os
import pandas as pd
import numpy as np
from datetime import datetime
import yaml

def load_raw_data():
    """Load raw data files."""
    raw_data_dir = "data/raw"
    processed_data_dir = "data/processed"
    
    # Create directories if they don't exist
    os.makedirs(raw_data_dir, exist_ok=True)
    os.makedirs(processed_data_dir, exist_ok=True)
    
    # Look for CSV files in raw data directory
    csv_files = [f for f in os.listdir(raw_data_dir) if f.endswith('.csv')]
    
    if not csv_files:
        print("No raw data files found. Generating sample data...")
        generate_sample_data(raw_data_dir)
        csv_files = [f for f in os.listdir(raw_data_dir) if f.endswith('.csv')]
    
    dataframes = {}
    for csv_file in csv_files:
        file_path = os.path.join(raw_data_dir, csv_file)
        print(f"Loading {file_path}")
        df = pd.read_csv(file_path)
        dataframes[csv_file] = df
        print(f"  Shape: {df.shape}")
    
    return dataframes

def generate_sample_data(raw_data_dir):
    """Generate sample datasets for demonstration."""
    print("Generating sample datasets...")
    
    # Generate customer data
    np.random.seed(42)
    n_customers = 1000
    
    customers = pd.DataFrame({
        'customer_id': range(1, n_customers + 1),
        'age': np.random.randint(18, 80, n_customers),
        'income': np.random.normal(50000, 20000, n_customers),
        'credit_score': np.random.randint(300, 850, n_customers),
        'region': np.random.choice(['North', 'South', 'East', 'West'], n_customers)
    })
    
    customer_file = os.path.join(raw_data_dir, 'customers.csv')
    customers.to_csv(customer_file, index=False)
    print(f"Generated {customer_file}")
    
    # Generate transaction data
    n_transactions = 5000
    transactions = pd.DataFrame({
        'transaction_id': range(1, n_transactions + 1),
        'customer_id': np.random.randint(1, n_customers + 1, n_transactions),
        'amount': np.random.exponential(100, n_transactions),
        'category': np.random.choice(['grocery', 'gas', 'restaurant', 'retail', 'online'], n_transactions),
        'timestamp': pd.date_range(start='2023-01-01', periods=n_transactions, freq='H')
    })
    
    transaction_file = os.path.join(raw_data_dir, 'transactions.csv')
    transactions.to_csv(transaction_file, index=False)
    print(f"Generated {transaction_file}")

def clean_data(dataframes):
    """Clean and validate the data."""
    print("\nCleaning data...")
    cleaned_data = {}
    
    for filename, df in dataframes.items():
        print(f"Cleaning {filename}")
        
        # Remove duplicates
        original_shape = df.shape
        df = df.drop_duplicates()
        print(f"  Removed {original_shape[0] - df.shape[0]} duplicates")
        
        # Handle missing values
        missing_before = df.isnull().sum().sum()
        df = df.fillna(df.mean(numeric_only=True))  # Fill numeric columns with mean
        df = df.fillna('Unknown')  # Fill categorical columns with 'Unknown'
        missing_after = df.isnull().sum().sum()
        print(f"  Handled {missing_before - missing_after} missing values")
        
        # Basic validation
        if 'amount' in df.columns:
            # Remove negative amounts
            negative_count = (df['amount'] < 0).sum()
            df = df[df['amount'] >= 0]
            print(f"  Removed {negative_count} negative amounts")
        
        cleaned_data[filename] = df
        print(f"  Final shape: {df.shape}")
    
    return cleaned_data

def feature_engineering(dataframes):
    """Create additional features."""
    print("\nPerforming feature engineering...")
    
    if 'customers.csv' in dataframes and 'transactions.csv' in dataframes:
        customers = dataframes['customers.csv']
        transactions = dataframes['transactions.csv']
        
        # Aggregate transaction features by customer
        transaction_features = transactions.groupby('customer_id').agg({
            'amount': ['sum', 'mean', 'count'],
            'category': lambda x: x.mode().iloc[0] if not x.empty else 'Unknown'
        }).round(2)
        
        # Flatten column names
        transaction_features.columns = ['total_spent', 'avg_transaction', 'transaction_count', 'top_category']
        transaction_features = transaction_features.reset_index()
        
        # Merge with customer data
        enriched_customers = customers.merge(transaction_features, on='customer_id', how='left')
        
        # Fill NaN values for customers with no transactions
        enriched_customers = enriched_customers.fillna(0)
        
        # Create risk score feature
        enriched_customers['risk_score'] = (
            (enriched_customers['credit_score'] - 300) / 550 * 0.4 +
            np.clip(enriched_customers['income'] / 100000, 0, 1) * 0.3 +
            np.clip(enriched_customers['transaction_count'] / 100, 0, 1) * 0.3
        ).round(3)
        
        dataframes['enriched_customers.csv'] = enriched_customers
        print(f"Created enriched customer dataset: {enriched_customers.shape}")
    
    return dataframes

def save_processed_data(dataframes):
    """Save processed data."""
    print("\nSaving processed data...")
    processed_dir = "data/processed"
    
    for filename, df in dataframes.items():
        # Use same filename but in processed directory
        output_path = os.path.join(processed_dir, filename)
        df.to_csv(output_path, index=False)
        print(f"Saved {output_path}")
    
    # Save processing metadata
    metadata = {
        'processed_at': datetime.now().isoformat(),
        'files_processed': list(dataframes.keys()),
        'total_records': sum(len(df) for df in dataframes.values())
    }
    
    metadata_path = os.path.join(processed_dir, 'processing_metadata.yaml')
    with open(metadata_path, 'w') as f:
        yaml.dump(metadata, f)
    print(f"Saved metadata to {metadata_path}")

def main():
    print(f"Starting data preprocessing pipeline at {datetime.now()}")
    
    # Load raw data
    raw_dataframes = load_raw_data()
    
    if not raw_dataframes:
        print("No data to process.")
        return
    
    # Clean data
    cleaned_dataframes = clean_data(raw_dataframes)
    
    # Feature engineering
    enriched_dataframes = feature_engineering(cleaned_dataframes)
    
    # Save processed data
    save_processed_data(enriched_dataframes)
    
    print("Data preprocessing completed successfully!")

if __name__ == "__main__":
    main()