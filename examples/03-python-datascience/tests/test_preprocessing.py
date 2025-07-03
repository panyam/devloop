#!/usr/bin/env python3
"""
Test cases for the data preprocessing pipeline
"""

import pytest
import tempfile
import os
import pandas as pd
import numpy as np
import sys

# Add src to path so we can import modules
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src', 'data'))

from preprocess import clean_data, feature_engineering, generate_sample_data

class TestDataCleaning:
    
    def test_remove_duplicates(self):
        """Test duplicate removal."""
        df = pd.DataFrame({
            'id': [1, 2, 2, 3, 4],
            'value': [10, 20, 20, 30, 40]
        })
        
        dataframes = {'test.csv': df}
        cleaned = clean_data(dataframes)
        
        assert cleaned['test.csv'].shape[0] == 4  # One duplicate removed
    
    def test_handle_missing_values(self):
        """Test missing value handling."""
        df = pd.DataFrame({
            'numeric_col': [1, 2, np.nan, 4, 5],
            'text_col': ['a', 'b', np.nan, 'd', 'e'],
            'amount': [10, 20, 30, 40, 50]
        })
        
        dataframes = {'test.csv': df}
        cleaned = clean_data(dataframes)
        
        result_df = cleaned['test.csv']
        
        # Should have no missing values
        assert result_df.isnull().sum().sum() == 0
        
        # Numeric column should be filled with mean
        assert not pd.isna(result_df.loc[2, 'numeric_col'])
        
        # Text column should be filled with 'Unknown'
        assert result_df.loc[2, 'text_col'] == 'Unknown'
    
    def test_remove_negative_amounts(self):
        """Test removal of negative amounts."""
        df = pd.DataFrame({
            'id': [1, 2, 3, 4, 5],
            'amount': [10, -5, 30, -2, 50]
        })
        
        dataframes = {'test.csv': df}
        cleaned = clean_data(dataframes)
        
        result_df = cleaned['test.csv']
        
        # Should only have positive amounts
        assert all(result_df['amount'] >= 0)
        assert result_df.shape[0] == 3  # 2 negative amounts removed

class TestFeatureEngineering:
    
    def test_customer_transaction_aggregation(self):
        """Test customer-transaction feature aggregation."""
        customers = pd.DataFrame({
            'customer_id': [1, 2, 3],
            'age': [25, 30, 35],
            'income': [50000, 60000, 70000]
        })
        
        transactions = pd.DataFrame({
            'transaction_id': [1, 2, 3, 4, 5],
            'customer_id': [1, 1, 2, 2, 3],
            'amount': [10, 20, 15, 25, 30],
            'category': ['grocery', 'gas', 'grocery', 'restaurant', 'retail']
        })
        
        dataframes = {
            'customers.csv': customers,
            'transactions.csv': transactions
        }
        
        result = feature_engineering(dataframes)
        
        assert 'enriched_customers.csv' in result
        enriched = result['enriched_customers.csv']
        
        # Should have additional columns
        expected_columns = ['customer_id', 'age', 'income', 'total_spent', 'avg_transaction', 'transaction_count', 'top_category', 'risk_score']
        assert all(col in enriched.columns for col in expected_columns)
        
        # Check aggregations for customer 1
        customer_1 = enriched[enriched['customer_id'] == 1].iloc[0]
        assert customer_1['total_spent'] == 30  # 10 + 20
        assert customer_1['avg_transaction'] == 15  # (10 + 20) / 2
        assert customer_1['transaction_count'] == 2
        assert customer_1['top_category'] == 'grocery'  # Could be grocery or gas
    
    def test_risk_score_calculation(self):
        """Test risk score calculation."""
        customers = pd.DataFrame({
            'customer_id': [1],
            'age': [30],
            'income': [50000],
            'credit_score': [650]
        })
        
        transactions = pd.DataFrame({
            'transaction_id': [1],
            'customer_id': [1],
            'amount': [100],
            'category': ['grocery']
        })
        
        dataframes = {
            'customers.csv': customers,
            'transactions.csv': transactions
        }
        
        result = feature_engineering(dataframes)
        enriched = result['enriched_customers.csv']
        
        # Risk score should be between 0 and 1
        risk_score = enriched.iloc[0]['risk_score']
        assert 0 <= risk_score <= 1
    
    def test_no_customer_transaction_data(self):
        """Test when customer or transaction data is missing."""
        # Only customers, no transactions
        customers = pd.DataFrame({
            'customer_id': [1, 2],
            'age': [25, 30]
        })
        
        dataframes = {'customers.csv': customers}
        result = feature_engineering(dataframes)
        
        # Should return original dataframes unchanged
        assert 'enriched_customers.csv' not in result
        assert result == dataframes

class TestSampleDataGeneration:
    
    def test_generate_sample_data(self):
        """Test sample data generation."""
        with tempfile.TemporaryDirectory() as temp_dir:
            generate_sample_data(temp_dir)
            
            # Check that files were created
            customers_file = os.path.join(temp_dir, 'customers.csv')
            transactions_file = os.path.join(temp_dir, 'transactions.csv')
            
            assert os.path.exists(customers_file)
            assert os.path.exists(transactions_file)
            
            # Check customer data
            customers = pd.read_csv(customers_file)
            assert customers.shape[0] == 1000
            expected_customer_cols = ['customer_id', 'age', 'income', 'credit_score', 'region']
            assert all(col in customers.columns for col in expected_customer_cols)
            
            # Check transaction data
            transactions = pd.read_csv(transactions_file)
            assert transactions.shape[0] == 5000
            expected_transaction_cols = ['transaction_id', 'customer_id', 'amount', 'category', 'timestamp']
            assert all(col in transactions.columns for col in expected_transaction_cols)
            
            # Check data quality
            assert customers['age'].min() >= 18
            assert customers['age'].max() <= 80
            assert customers['credit_score'].min() >= 300
            assert customers['credit_score'].max() <= 850

class TestDataValidation:
    
    def test_empty_dataframe_handling(self):
        """Test handling of empty DataFrames."""
        df = pd.DataFrame(columns=['id', 'value'])
        dataframes = {'empty.csv': df}
        
        cleaned = clean_data(dataframes)
        assert cleaned['empty.csv'].shape == (0, 2)
    
    def test_all_missing_data(self):
        """Test handling when all data is missing."""
        df = pd.DataFrame({
            'col1': [np.nan, np.nan, np.nan],
            'col2': [np.nan, np.nan, np.nan]
        })
        
        dataframes = {'test.csv': df}
        cleaned = clean_data(dataframes)
        
        result_df = cleaned['test.csv']
        
        # Should handle gracefully (fill with 'Unknown')
        assert result_df.isnull().sum().sum() == 0

if __name__ == "__main__":
    pytest.main([__file__, "-v"])