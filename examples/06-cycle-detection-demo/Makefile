# Cycle Detection Demo Makefile

.PHONY: run run-no-cycles run-mixed run-strict setup clean demo help

# Default target
help:
	@echo "Available targets:"
	@echo "  setup        - Create initial directory structure and sample files"
	@echo "  run          - Run with default cycle detection"
	@echo "  run-no-cycles - Run with cycle detection disabled"
	@echo "  run-mixed    - Run with mixed per-rule protection"
	@echo "  run-strict   - Run with strict cycle detection"
	@echo "  demo         - Run all configurations sequentially"
	@echo "  clean        - Clean generated files"

# Setup initial directory structure and sample files
setup:
	@echo "Creating directory structure..."
	mkdir -p backend/src backend/bin
	mkdir -p frontend/src frontend/dist
	mkdir -p input generated processed logs
	@echo "Creating sample files..."
	echo 'package main\n\nimport "fmt"\n\nfunc main() {\n    fmt.Println("Hello from backend!")\n}' > backend/src/main.go
	echo 'console.log("Hello from frontend!");' > frontend/src/app.js
	echo 'body { font-family: Arial, sans-serif; }' > frontend/src/style.css
	echo 'sample: true\ndata: "initial"' > input/sample.yaml
	@echo "Setup complete!"

# Run with default cycle detection
run: setup
	@echo "=== Running with DEFAULT cycle detection ==="
	@echo "Watch for cycle detection warnings during startup..."
	devloop -c .devloop.yaml

# Run with cycle detection disabled
run-no-cycles: setup
	@echo "=== Running with NO cycle detection ==="
	@echo "No cycle warnings should appear..."
	devloop -c .devloop-no-cycles.yaml

# Run with mixed protection
run-mixed: setup
	@echo "=== Running with MIXED cycle protection ==="
	@echo "Some rules protected, others not..."
	devloop -c .devloop-mixed-protection.yaml

# Run with strict cycle detection
run-strict: setup
	@echo "=== Running with STRICT cycle detection ==="
	@echo "Aggressive cycle detection settings..."
	devloop -c .devloop-strict.yaml

# Demo all configurations
demo:
	@echo "=== CYCLE DETECTION DEMO ==="
	@echo ""
	@echo "This demo will show cycle detection in action..."
	@echo "Press Ctrl+C to stop each configuration and move to the next"
	@echo ""
	@echo "1. First: Default configuration (with cycle detection)"
	@echo "   - Watch for warnings during startup"
	@echo "   - Try touching files to trigger cycles"
	@echo ""
	@read -p "Press Enter to continue..."
	$(MAKE) run-no-cycles || true
	@echo ""
	@echo "2. Second: No cycle detection"
	@echo "   - No warnings should appear"
	@echo "   - Cycles will run without protection"
	@echo ""
	@read -p "Press Enter to continue..."
	$(MAKE) run-mixed || true
	@echo ""
	@echo "3. Third: Mixed protection"
	@echo "   - Some rules protected, others not"
	@echo "   - Demonstrates per-rule overrides"
	@echo ""
	@read -p "Press Enter to continue..."
	$(MAKE) run-strict || true
	@echo ""
	@echo "Demo complete!"

# Clean generated files
clean:
	@echo "Cleaning generated files..."
	rm -rf backend/bin backend/src/build.log
	rm -rf frontend/dist frontend/src/build.log
	rm -rf generated processed logs
	@echo "Clean complete!"

# Test cycle detection without running devloop
test-static:
	@echo "Testing static cycle detection..."
	@echo "This will validate configurations without running:"
	@echo ""
	@echo "1. Default config:"
	devloop -c .devloop.yaml --validate || true
	@echo ""
	@echo "2. No cycles config:"
	devloop -c .devloop-no-cycles.yaml --validate || true
	@echo ""
	@echo "3. Mixed protection config:"
	devloop -c .devloop-mixed-protection.yaml --validate || true
	@echo ""
	@echo "4. Strict config:"
	devloop -c .devloop-strict.yaml --validate || true