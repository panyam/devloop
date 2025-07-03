# Python Data Science Project Example

This example demonstrates how to use `devloop` for a typical Python data science workflow, including Jupyter Lab development, model training, testing, and data preprocessing pipelines.

## Overview

This project showcases a complete machine learning pipeline with:

- **Data preprocessing** that triggers when raw data changes
- **Model training** that triggers when source code or configs change  
- **Automated testing** that runs when code is modified
- **Jupyter Lab** for interactive development and analysis

## Project Structure

```
03-python-datascience/
├── .devloop.yaml           # Devloop configuration
├── requirements.txt        # Python dependencies
├── README.md              # This file
├── Makefile               # Build automation
├── src/                   # Source code
│   ├── train.py           # Model training script
│   └── data/
│       └── preprocess.py  # Data preprocessing pipeline
├── tests/                 # Test suite
│   ├── test_training.py   # Training pipeline tests
│   └── test_preprocessing.py  # Data preprocessing tests
├── notebooks/             # Jupyter notebooks
│   ├── 01_data_exploration.ipynb
│   └── 02_model_evaluation.ipynb
├── configs/               # Configuration files
│   └── model.yaml         # Model training configuration
├── data/                  # Data storage
│   ├── raw/               # Raw data files
│   └── processed/         # Processed data files
├── models/                # Trained models and metrics
└── logs/                  # Log files from devloop rules
```

## Setup

### Prerequisites

- Python 3.8 or higher
- `devloop` installed ([installation guide](../../README.md#installation))

### Installation

1. **Navigate to the example directory:**
   ```bash
   cd examples/03-python-datascience
   ```

2. **Create a virtual environment (recommended):**
   ```bash
   python -m venv venv    # or use python3 
   source venv/bin/activate  # On Windows: venv\Scripts\activate
   ```

3. **Install dependencies:**
   ```bash
   pip install -r requirements.txt
   ```

## Running the Example

### Option 1: Using devloop (Recommended)

Start devloop to watch for file changes and automatically manage your development workflow:

```bash
devloop -c .devloop.yaml
```

This will start four concurrent processes:

1. **Jupyter Lab** (`jupyter` rule) - Interactive development environment
2. **Model Training** (`train` rule) - Automatic retraining when code/config changes
3. **Tests** (`test` rule) - Continuous testing when code changes
4. **Data Pipeline** (`pipeline` rule) - Data preprocessing when raw data changes

### Option 2: Manual Execution

You can also run individual components manually:

```bash
# Run data preprocessing
python src/data/preprocess.py

# Train model
python src/train.py --config configs/model.yaml

# Run tests
pytest tests/ -v

# Start Jupyter Lab
jupyter lab --no-browser --port=8888
```

## Workflow

### 1. Initial Setup
When you first run devloop, it will:
- Generate sample datasets in `data/raw/`
- Process the data and save to `data/processed/`
- Train an initial model and save to `models/`
- Start Jupyter Lab on port 8888
- Run the test suite

### 2. Development Cycle

**Modify Python source files** (in `src/`):
- Triggers model retraining
- Runs test suite
- Updates Jupyter Lab environment

**Modify configuration** (in `configs/`):
- Triggers model retraining with new parameters
- Saves new model and metrics

**Modify raw data** (in `data/raw/`):
- Triggers data preprocessing pipeline
- Updates processed datasets
- May trigger model retraining if using processed data

**Modify Jupyter notebooks** (in `notebooks/`):
- Restarts Jupyter Lab server
- Preserves notebook state and outputs

### 3. Monitoring

Each rule outputs logs with prefixes for easy identification:
- `[jupyter]` - Jupyter Lab server logs
- `[train]` - Model training progress and metrics
- `[test]` - Test execution results
- `[pipeline]` - Data preprocessing status

## Features Demonstrated

### 1. File Watching with Glob Patterns
```yaml
watch:
  - action: "include"
    patterns:
      - "src/**/*.py"       # All Python files in src/
      - "configs/**/*.yaml" # All YAML configs
  - action: "exclude"
    patterns:
      - "**/__pycache__/**" # Ignore Python cache
      - "**/*.pyc"          # Ignore compiled Python files
```

### 2. Sequential Command Execution
```yaml
commands:
  - "echo 'Starting model training...'"
  - "python src/train.py --config configs/model.yaml"
```

### 3. Working Directory Control
```yaml
workdir: "."  # Run commands from project root
```

### 4. Log Prefixing for Multi-Process Development
```yaml
settings:
  prefix_logs: true
  prefix_max_length: 10
```

## Customization

### Adding New Rules

To add a new development task, edit `.devloop.yaml`:

```yaml
rules:
  - name: "Code Formatting"
    prefix: "format"
    watch:
      - action: "include"
        patterns:
          - "src/**/*.py"
    commands:
      - "black src/"
      - "flake8 src/"
```

### Modifying Model Parameters

Edit `configs/model.yaml` to change model training parameters:

```yaml
model_params:
  n_estimators: 200      # Increase number of trees
  max_depth: 15          # Allow deeper trees
  random_state: 42       # Keep reproducible results
```

### Adding Dependencies

Add new packages to `requirements.txt` and reinstall:

```bash
echo "xgboost>=1.6.0" >> requirements.txt
pip install -r requirements.txt
```

## Example Workflows

### Data Science Exploration
1. Start devloop: `devloop -c .devloop.yaml`
2. Open Jupyter Lab: http://localhost:8888
3. Open `notebooks/01_data_exploration.ipynb`
4. Modify the notebook - Jupyter will restart automatically
5. Changes to `src/` files are reflected immediately in notebooks

### Model Development
1. Edit `src/train.py` to modify the training pipeline
2. Save the file - devloop automatically retrains the model
3. Check `models/metrics.yaml` for updated performance metrics
4. View results in `notebooks/02_model_evaluation.ipynb`

### Iterative Testing
1. Write new tests in `tests/`
2. Modify source code in `src/`
3. Tests run automatically on every change
4. Fix failures and see immediate feedback

### Data Pipeline Development
1. Add new raw data files to `data/raw/`
2. Preprocessing runs automatically
3. Check `data/processed/` for updated datasets
4. Model retraining may trigger if using processed data

## Integration with IDEs

This example works great with various development environments:

### VS Code
- Install Python extension
- Use integrated terminal to run `devloop`
- Edit files normally - devloop handles the rest

### PyCharm
- Open project directory
- Use terminal to run `devloop`
- Leverage PyCharm's debugging with running processes

### Vim/Emacs
- Run devloop in a tmux/screen session
- Edit files as usual
- Check devloop output for immediate feedback

## Performance Tips

1. **Exclude unnecessary files** from watching:
   ```yaml
   - action: "exclude"
     patterns:
       - "models/**"      # Don't watch model outputs
       - "logs/**"        # Don't watch log files
       - ".git/**"        # Don't watch git files
   ```

2. **Use specific patterns** instead of `**/*`:
   ```yaml
   - action: "include"
     patterns:
       - "src/**/*.py"    # Only Python files in src
   ```

3. **Optimize Jupyter startup** for faster restarts:
   ```bash
   jupyter lab --no-browser --port=8888 --NotebookApp.token=''
   ```

## Troubleshooting

### Common Issues

**Port 8888 already in use:**
```bash
# Find and kill existing Jupyter processes
lsof -ti:8888 | xargs kill -9
# Or use a different port in .devloop.yaml
```

**Module import errors:**
- Ensure virtual environment is activated
- Check that `src/` is in Python path
- Verify all dependencies are installed

**Model training fails:**
- Check that `configs/model.yaml` exists
- Verify data files are present
- Review training logs for specific errors

**Tests fail:**
- Run tests manually first: `pytest tests/ -v`
- Check test dependencies are installed
- Ensure test data is available

### Getting Help

1. Check devloop logs for error messages
2. Run individual commands manually to isolate issues
3. Verify file permissions and paths
4. Check Python environment and dependencies

## Next Steps

After exploring this example:

1. **Customize for your project**: Adapt the structure and configuration
2. **Add more rules**: Include linting, documentation generation, etc.
3. **Scale up**: Use devloop's agent/gateway mode for multi-project setups
4. **Integrate CI/CD**: Use similar patterns in your deployment pipeline

## Related Examples

- [Full-Stack Web Application](../01-fullstack-web/) - Multi-language development
- [Microservices](../02-microservices/) - Distributed development with gateway mode
- [Docker Integration](../06-docker-compose/) - Container-based development
