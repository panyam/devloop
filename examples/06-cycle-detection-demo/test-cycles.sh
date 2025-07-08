#!/bin/bash

# Test script to demonstrate cycle detection behavior
# This script triggers various file changes to show cycles in action

echo "=== Cycle Detection Demo Test Script ==="
echo "This script will create file changes to trigger cycles"
echo "Run this while devloop is running to see cycle detection in action"
echo ""

# Function to wait for user input
wait_for_input() {
    echo "Press Enter to continue..."
    read -r
}

# Test 1: Trigger self-referential logger
echo "Test 1: Triggering self-referential logger by creating a log file"
echo "This should trigger the 'self-ref-logger' rule, which will create more log files"
mkdir -p logs
echo "Test log entry" > logs/test.log
echo "✓ Created logs/test.log"
wait_for_input

# Test 2: Trigger backend builder
echo "Test 2: Triggering backend builder by modifying Go source"
echo "This will trigger the 'backend-builder' rule, which creates src/build.log"
echo "// Modified at $(date)" >> backend/src/main.go
echo "✓ Modified backend/src/main.go"
wait_for_input

# Test 3: Trigger cross-rule cycle
echo "Test 3: Triggering cross-rule cycle"
echo "Modifying input file to trigger config-generator -> config-processor cycle"
echo "updated_at: $(date)" >> input/sample.yaml
echo "✓ Modified input/sample.yaml"
wait_for_input

# Test 4: Trigger frontend builder
echo "Test 4: Triggering frontend builder by modifying JavaScript"
echo "This will trigger the 'frontend-builder' rule, which creates src/build.log"
echo "// Modified at $(date)" >> frontend/src/app.js
echo "✓ Modified frontend/src/app.js"
wait_for_input

# Test 5: Trigger log aggregator
echo "Test 5: Triggering log aggregator by creating multiple log files"
echo "This will trigger the 'log-aggregator' rule, which creates logs/aggregated.log"
echo "Log entry 1" > logs/entry1.log
echo "Log entry 2" > logs/entry2.log
echo "✓ Created multiple log files"
wait_for_input

# Test 6: Trigger test runner
echo "Test 6: Triggering test runner by modifying source files"
echo "This will trigger the 'test-runner' rule, which creates logs/test-results.log"
echo "// Test modification at $(date)" >> backend/src/main.go
echo "✓ Modified source files"
wait_for_input

echo "=== All tests completed! ==="
echo "Check devloop output for cycle detection warnings and behavior"
echo "You should see:"
echo "- Static warnings at startup for self-referential patterns"
echo "- Rules triggering themselves when they create watched files"
echo "- Cross-rule cycles when rules trigger each other"
echo "- Different behavior based on cycle protection settings"