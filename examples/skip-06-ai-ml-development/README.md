# AI/ML Development Pipeline Example

This example demonstrates using `devloop` for a comprehensive AI/ML development workflow with automated model training, experiment tracking, data pipelines, model serving, and MLOps practices.

## What's Included

- **Data Pipeline**: Automated data ingestion, validation, and preprocessing
- **Model Training**: Multiple ML frameworks (scikit-learn, PyTorch, TensorFlow)
- **Experiment Tracking**: MLflow for experiment management and model registry
- **Model Serving**: FastAPI service with real-time inference
- **Model Monitoring**: Data drift detection and model performance tracking
- **Jupyter Development**: Interactive model development and analysis
- **AutoML**: Automated hyperparameter tuning with Optuna
- **CI/CD Pipeline**: Automated testing, validation, and deployment

## Prerequisites

- Python 3.9+ with conda/virtualenv
- Docker and Docker Compose (for production deployment)
- devloop installed (`go install github.com/panyam/devloop@latest`)
- CUDA (optional, for GPU acceleration)

## Quick Start

1. **Setup environment:**
   ```bash
   make setup
   ```

2. **Start development environment:**
   ```bash
   make run
   # Or directly: devloop -c .devloop.yaml
   ```

3. **Access services:**
   - Jupyter Lab: http://localhost:8888
   - MLflow UI: http://localhost:5000
   - FastAPI Docs: http://localhost:8001/docs
   - Model Monitor: http://localhost:8002

## Architecture

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│    Data     │───▶│   Model     │───▶│   Model     │
│  Pipeline   │    │  Training   │    │  Serving    │
│             │    │             │    │             │
└─────────────┘    └─────────────┘    └─────────────┘
        │                 │                 │
        ▼                 ▼                 ▼
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│    Data     │    │  Experiment │    │    Model    │
│ Validation  │    │  Tracking   │    │ Monitoring  │
│             │    │  (MLflow)   │    │             │
└─────────────┘    └─────────────┘    └─────────────┘
```

## Development Workflow

### 1. Data-Driven Development
- **Data ingestion**: Monitors new data sources and triggers preprocessing
- **Data validation**: Runs data quality checks and schema validation
- **Feature engineering**: Automated feature extraction and transformation
- **Data versioning**: Tracks data lineage and versioning with DVC

### 2. Model Development Cycle
- **Jupyter development**: Interactive model exploration and prototyping
- **Automated training**: Triggers training when code or data changes
- **Hyperparameter tuning**: Automated optimization with Optuna
- **Model validation**: Comprehensive testing and validation pipeline

### 3. Experiment Management
- **MLflow tracking**: Automatic experiment logging and metrics tracking
- **Model registry**: Centralized model versioning and lifecycle management
- **Artifact storage**: Manages model artifacts, datasets, and outputs
- **Comparison tools**: Compare experiments and model performance

### 4. Production Pipeline
- **Model serving**: FastAPI service with automatic model loading
- **Performance monitoring**: Real-time inference monitoring and alerting
- **A/B testing**: Deploy multiple model versions for comparison
- **Drift detection**: Monitors data and concept drift in production

## Project Structure

```
06-ai-ml-development/
├── .devloop.yaml              # Devloop configuration
├── requirements.txt           # Python dependencies
├── environment.yml           # Conda environment
├── Makefile                  # Build automation
├── docker-compose.yml        # Production deployment
├── data/                     # Data storage
│   ├── raw/                  # Raw input data
│   ├── processed/            # Processed datasets
│   ├── features/             # Feature stores
│   └── external/             # External data sources
├── src/                      # Source code
│   ├── data/                 # Data pipeline modules
│   │   ├── ingestion.py      # Data collection
│   │   ├── validation.py     # Data quality checks
│   │   ├── preprocessing.py  # Data transformation
│   │   └── features.py       # Feature engineering
│   ├── models/               # Model implementations
│   │   ├── sklearn_models.py # Scikit-learn models
│   │   ├── pytorch_models.py # PyTorch models
│   │   ├── tensorflow_models.py # TensorFlow models
│   │   └── ensemble.py       # Ensemble methods
│   ├── training/             # Training pipelines
│   │   ├── train.py          # Main training script
│   │   ├── hyperopt.py       # Hyperparameter optimization
│   │   ├── validation.py     # Model validation
│   │   └── registry.py       # Model registration
│   ├── serving/              # Model serving
│   │   ├── api.py            # FastAPI service
│   │   ├── inference.py      # Inference logic
│   │   └── monitoring.py     # Performance monitoring
│   └── utils/                # Utility functions
│       ├── config.py         # Configuration management
│       ├── logging.py        # Logging setup
│       └── metrics.py        # Custom metrics
├── notebooks/                # Jupyter notebooks
│   ├── 01_data_exploration.ipynb
│   ├── 02_feature_engineering.ipynb
│   ├── 03_model_development.ipynb
│   ├── 04_experiment_analysis.ipynb
│   └── 05_model_interpretation.ipynb
├── tests/                    # Test suite
│   ├── unit/                 # Unit tests
│   ├── integration/          # Integration tests
│   └── performance/          # Performance tests
├── configs/                  # Configuration files
│   ├── model_configs/        # Model configurations
│   ├── training_configs/     # Training configurations
│   └── deployment_configs/   # Deployment configurations
├── models/                   # Trained models
│   ├── artifacts/            # Model artifacts
│   ├── checkpoints/          # Training checkpoints
│   └── exports/              # Exported models
├── monitoring/               # Monitoring setup
│   ├── dashboards/           # Grafana dashboards
│   ├── alerts/               # Alert configurations
│   └── metrics/              # Custom metrics
└── logs/                     # Application logs
```

## Key Features

### 1. Multi-Framework Support
```python
# PyTorch implementation
class PyTorchModel(nn.Module):
    def __init__(self, input_size, hidden_size, output_size):
        super().__init__()
        self.layers = nn.Sequential(
            nn.Linear(input_size, hidden_size),
            nn.ReLU(),
            nn.Linear(hidden_size, output_size)
        )

# TensorFlow implementation  
class TensorFlowModel(tf.keras.Model):
    def __init__(self, input_size, hidden_size, output_size):
        super().__init__()
        self.dense1 = tf.keras.layers.Dense(hidden_size, activation='relu')
        self.dense2 = tf.keras.layers.Dense(output_size)
```

### 2. Automated Experiment Tracking
```python
import mlflow
import mlflow.sklearn

with mlflow.start_run():
    # Train model
    model = train_model(X_train, y_train)
    
    # Log parameters
    mlflow.log_params(model_params)
    
    # Log metrics
    mlflow.log_metrics({
        'accuracy': accuracy,
        'precision': precision,
        'recall': recall
    })
    
    # Log model
    mlflow.sklearn.log_model(model, "model")
```

### 3. Real-time Model Serving
```python
from fastapi import FastAPI
from pydantic import BaseModel

app = FastAPI()

class PredictionRequest(BaseModel):
    features: List[float]

@app.post("/predict")
async def predict(request: PredictionRequest):
    prediction = model.predict([request.features])
    return {"prediction": prediction[0]}
```

### 4. Data Drift Detection
```python
from evidently import ColumnMapping
from evidently.report import Report
from evidently.metric_preset import DataDriftPreset

report = Report(metrics=[DataDriftPreset()])
report.run(reference_data=reference_df, current_data=current_df)
```

## Configuration Examples

### Model Configuration (`configs/model_configs/xgboost.yaml`)
```yaml
model_type: "xgboost"
parameters:
  n_estimators: 100
  max_depth: 6
  learning_rate: 0.1
  subsample: 0.8
  colsample_bytree: 0.8
  random_state: 42

training:
  validation_split: 0.2
  early_stopping_rounds: 10
  eval_metric: "logloss"

hyperopt:
  trials: 100
  search_space:
    n_estimators: [50, 500]
    max_depth: [3, 10]
    learning_rate: [0.01, 0.3]
```

### Data Pipeline Configuration (`configs/data_pipeline.yaml`)
```yaml
data_sources:
  - name: "customer_data"
    type: "csv"
    path: "data/raw/customers.csv"
    schema: "schemas/customer_schema.json"
  
  - name: "transaction_data"
    type: "database"
    connection: "postgresql://localhost:5432/transactions"
    query: "SELECT * FROM transactions WHERE date >= '{start_date}'"

preprocessing:
  steps:
    - name: "missing_values"
      method: "impute"
      strategy: "median"
    
    - name: "outliers"
      method: "clip"
      bounds: [0.01, 0.99]
    
    - name: "scaling"
      method: "standard"

features:
  numerical: ["age", "income", "credit_score"]
  categorical: ["category", "region", "segment"]
  target: "churn"
```

## Development Commands

```bash
# Data pipeline
make data-pipeline      # Run complete data pipeline
make data-validate      # Validate data quality
make data-profile       # Generate data profiling report

# Model training
make train-all          # Train all configured models
make train-sklearn      # Train scikit-learn models
make train-pytorch      # Train PyTorch models
make train-tensorflow   # Train TensorFlow models

# Hyperparameter optimization
make hyperopt           # Run hyperparameter optimization
make hyperopt-parallel  # Run parallel optimization

# Model evaluation
make evaluate           # Evaluate all models
make compare-models     # Compare model performance
make model-report       # Generate model evaluation report

# Model serving
make serve-model        # Start model serving API
make test-api          # Test API endpoints
make load-test         # Run load testing

# Monitoring
make monitor-start     # Start monitoring services
make monitor-data      # Check for data drift
make monitor-model     # Check model performance

# Jupyter
make jupyter           # Start Jupyter Lab
make jupyter-trust     # Trust all notebooks

# MLflow
make mlflow-ui         # Start MLflow UI
make mlflow-clean      # Clean MLflow experiments
```

## Try It Out

### 1. Data Pipeline Development
```bash
# Add new data source
echo "new_data.csv" > data/raw/new_source.csv

# Watch automatic data validation and preprocessing
# Check logs for pipeline execution
tail -f logs/data-pipeline.log
```

### 2. Model Development
```bash
# Modify model architecture
vim src/models/pytorch_models.py

# Watch automatic retraining and evaluation
# Check MLflow UI for new experiments
```

### 3. Experiment Tracking
```bash
# Compare different models
mlflow ui

# View experiment results
mlflow experiments list
mlflow runs list --experiment-id 1
```

### 4. Model Serving
```bash
# Test API endpoint
curl -X POST "http://localhost:8001/predict" \
  -H "Content-Type: application/json" \
  -d '{"features": [1.0, 2.0, 3.0]}'

# Monitor performance
curl http://localhost:8002/metrics
```

## Advanced Features

### 1. Automated ML Pipeline
```python
from sklearn.pipeline import Pipeline
from sklearn.compose import ColumnTransformer
from sklearn.preprocessing import StandardScaler, OneHotEncoder

# Automated feature preprocessing
preprocessor = ColumnTransformer(
    transformers=[
        ('num', StandardScaler(), numerical_features),
        ('cat', OneHotEncoder(), categorical_features)
    ]
)

# Complete ML pipeline
pipeline = Pipeline([
    ('preprocessor', preprocessor),
    ('classifier', model)
])
```

### 2. Model Interpretability
```python
import shap
import lime

# SHAP explanations
explainer = shap.TreeExplainer(model)
shap_values = explainer.shap_values(X_test)

# LIME explanations
lime_explainer = lime.lime_tabular.LimeTabularExplainer(
    X_train.values,
    feature_names=feature_names,
    class_names=class_names
)
```

### 3. Distributed Training
```python
import ray
from ray import tune

# Ray Tune for hyperparameter optimization
config = {
    "learning_rate": tune.loguniform(1e-4, 1e-1),
    "batch_size": tune.choice([16, 32, 64, 128]),
    "hidden_size": tune.choice([64, 128, 256])
}

analysis = tune.run(
    train_model,
    config=config,
    num_samples=100,
    resources_per_trial={"cpu": 2, "gpu": 1}
)
```

### 4. Model Deployment
```yaml
# Kubernetes deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ml-model-serving
spec:
  replicas: 3
  selector:
    matchLabels:
      app: ml-model
  template:
    metadata:
      labels:
        app: ml-model
    spec:
      containers:
      - name: model-server
        image: ml-model:latest
        ports:
        - containerPort: 8001
        env:
        - name: MODEL_PATH
          value: "/models/production"
```

## MLOps Best Practices

### 1. Model Versioning
- Semantic versioning for models
- Git-based code versioning
- Data versioning with DVC
- Artifact tracking with MLflow

### 2. Testing Strategy
- Unit tests for data processing
- Integration tests for pipelines
- Model performance tests
- Data quality validation tests

### 3. Monitoring and Alerting
- Data drift detection
- Model performance monitoring
- Infrastructure monitoring
- Automated alerting systems

### 4. CI/CD Pipeline
```yaml
# .github/workflows/ml-pipeline.yml
name: ML Pipeline
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Setup Python
        uses: actions/setup-python@v4
        with:
          python-version: '3.9'
      - name: Install dependencies
        run: pip install -r requirements.txt
      - name: Run tests
        run: pytest tests/
      - name: Validate data
        run: python src/data/validation.py
      - name: Train model
        run: python src/training/train.py
      - name: Evaluate model
        run: python src/training/validation.py
```

## Troubleshooting

**Environment issues:**
```bash
# Reset conda environment
conda env remove -n ml-dev
make setup

# Check GPU availability
python -c "import torch; print(torch.cuda.is_available())"
```

**MLflow tracking issues:**
```bash
# Reset MLflow tracking
rm -rf mlruns/
mlflow server --backend-store-uri sqlite:///mlflow.db

# Check MLflow connection
mlflow experiments list
```

**Model serving issues:**
```bash
# Check API health
curl http://localhost:8001/health

# View API logs
docker logs ml-serving

# Test with sample data
python scripts/test_api.py
```

**Data pipeline issues:**
```bash
# Validate data schemas
python src/data/validation.py --check-schema

# Check data quality
python src/data/validation.py --profile

# Manual pipeline run
python src/data/pipeline.py --force
```

## Extensions

### Adding New Models
```python
# src/models/custom_model.py
class CustomModel:
    def __init__(self, config):
        self.config = config
    
    def train(self, X, y):
        # Implementation
        pass
    
    def predict(self, X):
        # Implementation
        pass
```

### Custom Metrics
```python
# src/utils/custom_metrics.py
def business_metric(y_true, y_pred):
    # Custom business logic
    return metric_value

# Register with MLflow
mlflow.log_metric("business_metric", business_metric(y_test, y_pred))
```

### New Data Sources
```python
# src/data/connectors/custom_connector.py
class CustomDataConnector:
    def __init__(self, config):
        self.config = config
    
    def extract(self):
        # Data extraction logic
        return data
```

## Next Steps

- Explore advanced AutoML frameworks (AutoGluon, H2O)
- Implement federated learning
- Add real-time streaming ML pipelines
- Integrate with cloud ML platforms (AWS SageMaker, GCP Vertex AI)
- Implement MLOps with Kubeflow or MLflow
- Add advanced monitoring with Evidently or WhyLabs