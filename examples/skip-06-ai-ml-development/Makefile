.PHONY: setup run deps clean help

# Default target
run: deps
	@echo "Starting AI/ML development environment with devloop..."
	devloop -c .devloop.yaml

# Setup complete development environment
setup:
	@echo "Setting up AI/ML development environment..."
	@if ! command -v python >/dev/null 2>&1; then \
		echo "Error: Python is not installed"; \
		exit 1; \
	fi
	@if ! command -v conda >/dev/null 2>&1; then \
		echo "Warning: Conda not found, using pip instead"; \
		$(MAKE) setup-pip; \
	else \
		$(MAKE) setup-conda; \
	fi
	@echo "Creating required directories..."
	@mkdir -p data/{raw,processed,features,external,monitoring}
	@mkdir -p models/{artifacts,checkpoints,exports}
	@mkdir -p logs monitoring/dashboards configs/{model_configs,training_configs,deployment_configs}
	@echo "Setup completed successfully!"

# Setup with conda
setup-conda:
	@echo "Creating conda environment..."
	conda env create -f environment.yml || conda env update -f environment.yml
	@echo "Activating environment and installing additional packages..."
	conda activate ml-dev && pip install -r requirements-dev.txt

# Setup with pip
setup-pip:
	@echo "Installing Python dependencies with pip..."
	pip install -r requirements.txt
	pip install -r requirements-dev.txt

# Install dependencies only
deps:
	@echo "Installing/updating dependencies..."
	@if [ -f environment.yml ] && command -v conda >/dev/null 2>&1; then \
		conda env update -f environment.yml; \
	else \
		pip install -r requirements.txt; \
	fi

# Data pipeline commands
data-pipeline:
	@echo "Running complete data pipeline..."
	python src/data/pipeline.py

data-validate:
	@echo "Validating data quality..."
	python src/data/validation.py --check-schema --profile

data-profile:
	@echo "Generating data profiling report..."
	python src/data/profiling.py

# Model training commands
train-all:
	@echo "Training all configured models..."
	python src/training/train.py --all-models

train-sklearn:
	@echo "Training scikit-learn models..."
	python src/training/train.py --framework sklearn

train-pytorch:
	@echo "Training PyTorch models..."
	python src/training/train.py --framework pytorch

train-tensorflow:
	@echo "Training TensorFlow models..."
	python src/training/train.py --framework tensorflow

# Hyperparameter optimization
hyperopt:
	@echo "Running hyperparameter optimization..."
	python src/training/hyperopt.py

hyperopt-parallel:
	@echo "Running parallel hyperparameter optimization..."
	python src/training/hyperopt.py --parallel --n-jobs 4

# Model evaluation
evaluate:
	@echo "Evaluating all models..."
	python src/training/validation.py --evaluate-all

compare-models:
	@echo "Comparing model performance..."
	python src/training/comparison.py

model-report:
	@echo "Generating model evaluation report..."
	python src/reporting/model_report.py

# Model serving
serve-model:
	@echo "Starting model serving API..."
	cd src/serving && python api.py

test-api:
	@echo "Testing API endpoints..."
	python tests/test_api.py

load-test:
	@echo "Running load testing..."
	python tests/load_test.py

# Monitoring
monitor-start:
	@echo "Starting monitoring services..."
	python src/monitoring/drift_detector.py &
	python src/monitoring/performance_monitor.py &

monitor-data:
	@echo "Checking for data drift..."
	python src/monitoring/drift_detector.py --check-drift

monitor-model:
	@echo "Checking model performance..."
	python src/monitoring/performance_monitor.py --check-performance

# Jupyter
jupyter:
	@echo "Starting Jupyter Lab..."
	jupyter lab --no-browser --port=8888

jupyter-trust:
	@echo "Trusting all notebooks..."
	find notebooks -name "*.ipynb" -exec jupyter trust {} \;

# MLflow
mlflow-ui:
	@echo "Starting MLflow UI..."
	mlflow ui --host 0.0.0.0 --port 5000

mlflow-clean:
	@echo "Cleaning MLflow experiments..."
	rm -rf mlruns/ mlflow.db

# Development commands
dev-notebook:
	@echo "Starting notebook development environment..."
	jupyter lab --no-browser --port=8888 &
	mlflow ui --host 0.0.0.0 --port 5000 &

dev-training:
	@echo "Starting training development environment..."
	python src/training/train.py --watch

dev-serving:
	@echo "Starting serving development environment..."
	cd src/serving && python api.py --reload

# Testing
test:
	@echo "Running all tests..."
	pytest tests/ -v

test-unit:
	@echo "Running unit tests..."
	pytest tests/unit/ -v

test-integration:
	@echo "Running integration tests..."
	pytest tests/integration/ -v

test-performance:
	@echo "Running performance tests..."
	pytest tests/performance/ -v

test-coverage:
	@echo "Running tests with coverage..."
	pytest tests/ --cov=src --cov-report=html

# Code quality
lint:
	@echo "Running linters..."
	flake8 src tests
	black --check src tests
	isort --check-only src tests

format:
	@echo "Formatting code..."
	black src tests
	isort src tests

type-check:
	@echo "Running type checking..."
	mypy src --ignore-missing-imports

# Documentation
docs:
	@echo "Generating documentation..."
	sphinx-build -b html docs/ docs/_build/

docs-serve:
	@echo "Serving documentation..."
	cd docs/_build && python -m http.server 8080

# Cleanup
clean:
	@echo "Cleaning build artifacts..."
	find . -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
	find . -type f -name "*.pyc" -delete
	find . -type f -name "*.pyo" -delete
	find . -type d -name "*.egg-info" -exec rm -rf {} + 2>/dev/null || true
	rm -rf .pytest_cache
	rm -rf .coverage htmlcov
	rm -rf dist build

clean-data:
	@echo "Cleaning processed data..."
	rm -rf data/processed/*
	rm -rf data/features/*

clean-models:
	@echo "Cleaning trained models..."
	rm -rf models/artifacts/*
	rm -rf models/checkpoints/*

clean-all: clean clean-data clean-models mlflow-clean
	@echo "Deep clean completed!"

# Docker commands
docker-build:
	@echo "Building Docker images..."
	docker build -t ml-training -f docker/Dockerfile.training .
	docker build -t ml-serving -f docker/Dockerfile.serving .

docker-run:
	@echo "Starting services with Docker Compose..."
	docker-compose up -d

docker-stop:
	@echo "Stopping Docker services..."
	docker-compose down

# Health checks
health:
	@echo "Checking system health..."
	@echo "Python version: $(shell python --version)"
	@echo "MLflow status:" && mlflow --version
	@echo "Jupyter status:" && jupyter --version
	@echo "PyTorch status:" && python -c "import torch; print(f'PyTorch {torch.__version__}')"
	@echo "TensorFlow status:" && python -c "import tensorflow as tf; print(f'TensorFlow {tf.__version__}')"
	@echo "CUDA available:" && python -c "import torch; print(torch.cuda.is_available())"

# Configuration validation
validate-config:
	@echo "Validating configuration files..."
	python -c "import yaml; [yaml.safe_load(open(f)) for f in ['configs/data_pipeline.yaml']]"

# Environment export
export-env:
	@echo "Exporting environment..."
	@if command -v conda >/dev/null 2>&1; then \
		conda env export > environment.yml; \
	fi
	pip freeze > requirements.txt

# Show help
help:
	@echo "AI/ML Development Commands:"
	@echo ""
	@echo "Setup & Environment:"
	@echo "  make setup       - Complete environment setup"
	@echo "  make deps        - Install/update dependencies"
	@echo "  make run         - Start development environment with devloop"
	@echo ""
	@echo "Data Pipeline:"
	@echo "  make data-pipeline    - Run complete data pipeline"
	@echo "  make data-validate    - Validate data quality"
	@echo "  make data-profile     - Generate data profiling report"
	@echo ""
	@echo "Model Training:"
	@echo "  make train-all        - Train all configured models"
	@echo "  make train-sklearn    - Train scikit-learn models"
	@echo "  make train-pytorch    - Train PyTorch models"
	@echo "  make train-tensorflow - Train TensorFlow models"
	@echo "  make hyperopt         - Run hyperparameter optimization"
	@echo ""
	@echo "Model Evaluation:"
	@echo "  make evaluate         - Evaluate all models"
	@echo "  make compare-models   - Compare model performance"
	@echo "  make model-report     - Generate model evaluation report"
	@echo ""
	@echo "Model Serving:"
	@echo "  make serve-model      - Start model serving API"
	@echo "  make test-api         - Test API endpoints"
	@echo "  make load-test        - Run load testing"
	@echo ""
	@echo "Development:"
	@echo "  make jupyter          - Start Jupyter Lab"
	@echo "  make mlflow-ui        - Start MLflow UI"
	@echo "  make dev-notebook     - Start notebook development environment"
	@echo ""
	@echo "Testing & Quality:"
	@echo "  make test             - Run all tests"
	@echo "  make lint             - Run code linters"
	@echo "  make format           - Format code"
	@echo "  make type-check       - Run type checking"
	@echo ""
	@echo "Monitoring:"
	@echo "  make monitor-start    - Start monitoring services"
	@echo "  make monitor-data     - Check for data drift"
	@echo "  make monitor-model    - Check model performance"
	@echo ""
	@echo "Utilities:"
	@echo "  make clean            - Clean build artifacts"
	@echo "  make health           - Check system health"
	@echo "  make help             - Show this help message"