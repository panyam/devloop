# Makefile for Python Data Science Example
# This provides convenient commands for development and testing

.PHONY: help setup clean test train preprocess jupyter lint format install run-devloop demo

# Default target
help:
	@echo "Python Data Science Example - Available Commands:"
	@echo ""
	@echo "Setup and Installation:"
	@echo "  setup          - Create virtual environment and install dependencies"
	@echo "  install        - Install Python dependencies only"
	@echo "  clean          - Clean up generated files and directories"
	@echo ""
	@echo "Development:"
	@echo "  run-devloop    - Start devloop to watch for changes"
	@echo "  jupyter        - Start Jupyter Lab manually"
	@echo "  train          - Train model manually"
	@echo "  preprocess     - Run data preprocessing manually"
	@echo "  test           - Run test suite"
	@echo ""
	@echo "Code Quality:"
	@echo "  lint           - Run code linting (if flake8 is installed)"
	@echo "  format         - Format code (if black is installed)"
	@echo ""
	@echo "Demo:"
	@echo "  demo           - Run full demo workflow"
	@echo ""
	@echo "For normal development, just run: make run-devloop"

# Setup virtual environment and install dependencies
setup:
	@echo "Setting up Python Data Science example..."
	python -m venv venv
	@echo "Virtual environment created. Activate with:"
	@echo "  source venv/bin/activate  (Linux/Mac)"
	@echo "  venv\\Scripts\\activate     (Windows)"
	@echo ""
	@echo "Then run: make install"

# Install dependencies
install:
	@echo "Installing Python dependencies..."
	pip install -r requirements.txt
	@echo "Dependencies installed successfully!"

# Clean up generated files
clean:
	@echo "Cleaning up generated files..."
	rm -rf data/processed/*
	rm -rf models/*
	rm -rf logs/*
	rm -rf **/__pycache__
	rm -rf **/*.pyc
	rm -rf .pytest_cache
	rm -rf **/.ipynb_checkpoints
	@echo "Cleanup complete!"

# Run tests
test:
	@echo "Running test suite..."
	pytest tests/ -v --color=yes --tb=short

# Train model manually
train:
	@echo "Training model..."
	python src/train.py --config configs/model.yaml

# Run data preprocessing
preprocess:
	@echo "Running data preprocessing pipeline..."
	python src/data/preprocess.py

# Start Jupyter Lab
jupyter:
	@echo "Starting Jupyter Lab..."
	@echo "Open http://localhost:8888 in your browser"
	jupyter lab --no-browser --port=8888 --ip=0.0.0.0 --allow-root

# Run devloop (main development command)
run-devloop:
	@echo "Starting devloop for Python Data Science development..."
	@echo "This will start:"
	@echo "  - Jupyter Lab on port 8888"
	@echo "  - Model training on code changes"
	@echo "  - Test runner on code changes"
	@echo "  - Data preprocessing on data changes"
	@echo ""
	@echo "Press Ctrl+C to stop all processes"
	devloop -c .devloop.yaml

# Code formatting (optional)
format:
	@echo "Formatting Python code..."
	@if command -v black >/dev/null 2>&1; then \
		black src/ tests/ --line-length 88; \
		echo "Code formatted with black!"; \
	else \
		echo "black not installed. Install with: pip install black"; \
	fi

# Code linting (optional)
lint:
	@echo "Running code linting..."
	@if command -v flake8 >/dev/null 2>&1; then \
		flake8 src/ tests/ --max-line-length 88 --ignore E203,W503; \
		echo "Linting complete!"; \
	else \
		echo "flake8 not installed. Install with: pip install flake8"; \
	fi

# Demo workflow
demo:
	@echo "Running Python Data Science Demo..."
	@echo ""
	@echo "Step 1: Data Preprocessing"
	make preprocess
	@echo ""
	@echo "Step 2: Model Training"
	make train
	@echo ""
	@echo "Step 3: Running Tests"
	make test
	@echo ""
	@echo "Demo complete! Check the following:"
	@echo "  - Processed data: data/processed/"
	@echo "  - Trained model: models/"
	@echo "  - Test results: (shown above)"
	@echo ""
	@echo "To start interactive development:"
	@echo "  make run-devloop"

# Check if devloop is installed
check-devloop:
	@if ! command -v devloop >/dev/null 2>&1; then \
		echo "Error: devloop not found in PATH"; \
		echo "Please install devloop first:"; \
		echo "  go install github.com/panyam/devloop@latest"; \
		exit 1; \
	fi

# Override run-devloop to check for devloop
run-devloop: check-devloop
	@$(MAKE) run-devloop-impl

run-devloop-impl:
	@echo "Starting devloop for Python Data Science development..."
	@echo "This will start:"
	@echo "  - Jupyter Lab on port 8888"
	@echo "  - Model training on code changes"
	@echo "  - Test runner on code changes"
	@echo "  - Data preprocessing on data changes"
	@echo ""
	@echo "Press Ctrl+C to stop all processes"
	devloop -c .devloop.yaml