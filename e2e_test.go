package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEndToEnd(t *testing.T) {
	withTestContext(t, 10*time.Second, func(t *testing.T, tmpDir string) {
		// Define paths within the temporary directory
		multiYamlPath := filepath.Join(tmpDir, ".devloop.yaml")
		triggerFilePath := filepath.Join(tmpDir, "trigger.txt")
		outputFilePath := filepath.Join(tmpDir, "output.txt")

		// Unique string to verify command execution
		uniqueString := "command_executed_" + time.Now().Format("20060102150405")

		// Create .devloop.yaml content
		multiYamlContent := fmt.Sprintf(`
rules:
  - name: "E2E Test Rule"
    watch:
      - action: include
        patterns:
          - "%s"
    commands:
      - "echo %s > %s"
`, filepath.Base(triggerFilePath), uniqueString, outputFilePath)

		// Write .devloop.yaml
		err := os.WriteFile(multiYamlPath, []byte(multiYamlContent), 0644)
		assert.NoError(t, err)

		// 2. Run Orchestrator
		orchestrator, err := NewOrchestrator(multiYamlPath, "")
		assert.NoError(t, err)
		assert.NotNil(t, orchestrator)

		// Start the orchestrator in a goroutine
		go func() {
			err := orchestrator.Start()
			assert.NoError(t, err)
		}()
		defer orchestrator.Stop()

		// Give the watcher some time to initialize
		time.Sleep(500 * time.Millisecond)

		// 3. Trigger and Verify
		// Create the trigger file
		err = os.WriteFile(triggerFilePath, []byte("trigger"), 0644)
		assert.NoError(t, err)

		// Poll for the output file to appear and contain the unique string
		timeout := time.After(5 * time.Second)
		for {
			select {
			case <-timeout:
				t.Fatal("Timeout waiting for output file")
			default:
				content, readErr := os.ReadFile(outputFilePath)
				if readErr == nil && strings.TrimSpace(string(content)) == uniqueString {
					return // Test success
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	})
}

func TestDebouncing(t *testing.T) {
	withTestContext(t, 0*time.Second, func(t *testing.T, tmpDir string) {
		// Define paths within the temporary directory
		multiYamlPath := filepath.Join(tmpDir, ".devloop.yaml")
		triggerFilePath := filepath.Join(tmpDir, "trigger_debounce.txt")
		outputFilePath := filepath.Join(tmpDir, "output_debounce.txt")

		// Create .devloop.yaml content to append to output file
		multiYamlContent := fmt.Sprintf(`
rules:
  - name: "Debounce Test Rule"
    watch:
      - action: include
        patterns:
          - "%s"
    commands:
      - "echo 'executed' >> %s"
`, filepath.Base(triggerFilePath), outputFilePath)

		// Write .devloop.yaml
		err := os.WriteFile(multiYamlPath, []byte(multiYamlContent), 0644)
		assert.NoError(t, err)

		// 2. Run Orchestrator
		orchestrator, err := NewOrchestrator(multiYamlPath, "")
		assert.NoError(t, err)
		assert.NotNil(t, orchestrator)

		// Set a shorter debounce duration for testing
		orchestrator.debounceDuration = 200 * time.Millisecond

		// Start the orchestrator in a goroutine
		go func() {
			err := orchestrator.Start()
			assert.NoError(t, err)
		}()
		defer orchestrator.Stop()

		// Give the watcher some time to initialize
		time.Sleep(500 * time.Millisecond)

		// 3. Trigger rapidly multiple times
		for i := range 5 {
			err = os.WriteFile(triggerFilePath, []byte(fmt.Sprintf("trigger %d", i)), 0644)
			assert.NoError(t, err)
			time.Sleep(50 * time.Millisecond) // Rapid writes within debounce duration
		}

		// Wait for longer than debounce duration to ensure command executes once
		time.Sleep(orchestrator.debounceDuration + 10000*time.Millisecond)

		// 4. Verify command execution count
		content, readErr := os.ReadFile(outputFilePath)
		assert.NoError(t, readErr)

		lines := strings.Split(strings.TrimSpace(string(content)), "\n")
		assert.Len(t, lines, 1, "Command should have executed only once due to debouncing")
		assert.Equal(t, "executed", lines[0])
	})
}

func TestProcessManagement(t *testing.T) {
	withTestContext(t, 10*time.Second, func(t *testing.T, tmpDir string) {
		// Define paths within the temporary directory
		multiYamlPath := filepath.Join(tmpDir, ".devloop.yaml")
		triggerFilePath := filepath.Join(tmpDir, "trigger_process.txt")
		heartbeatFilePath := filepath.Join(tmpDir, "heartbeat.txt")

		// Create .devloop.yaml content with a long-running command
		multiYamlContent := fmt.Sprintf(`
rules:
  - name: "Process Test Rule"
    watch:
      - action: include
        patterns:
          - "%s"
    commands:
      - "bash -c 'ID=$(date +%%%%s%%%%N); echo \"Heartbeat $ID\" >> %s; while true; do echo \"Heartbeat $ID\" >> %s; sleep 0.1; done'"
`, filepath.Base(triggerFilePath), heartbeatFilePath, heartbeatFilePath)

		// Write .devloop.yaml
		err := os.WriteFile(multiYamlPath, []byte(multiYamlContent), 0644)
		assert.NoError(t, err)

		// 2. Run Orchestrator
		orchestrator, err := NewOrchestrator(multiYamlPath, "")
		assert.NoError(t, err)
		assert.NotNil(t, orchestrator)

		// Set a shorter debounce duration for testing
		orchestrator.debounceDuration = 200 * time.Millisecond

		// Start the orchestrator in a goroutine
		go func() {
			err := orchestrator.Start()
			assert.NoError(t, err)
		}()
		defer orchestrator.Stop()

		// Give the watcher some time to initialize
		time.Sleep(500 * time.Millisecond)

		// 3. Trigger rapidly multiple times
		for i := range 3 {
			err = os.WriteFile(triggerFilePath, []byte(fmt.Sprintf("trigger %d", i)), 0644)
			assert.NoError(t, err)
			time.Sleep(orchestrator.debounceDuration / 2) // Trigger within debounce duration
		}

		// Wait for a bit longer to allow processes to start and heartbeats to write
		time.Sleep(orchestrator.debounceDuration * 2)

		// 4. Verify process termination
		content, readErr := os.ReadFile(heartbeatFilePath)
		assert.NoError(t, readErr)

		// Count unique process IDs in the heartbeat file
		heartbeats := strings.Split(strings.TrimSpace(string(content)), "\n")
		activeProcesses := make(map[string]struct{})
		for _, line := range heartbeats {
			if strings.HasPrefix(line, "Heartbeat ") {
				id := strings.TrimPrefix(line, "Heartbeat ")
				activeProcesses[id] = struct{}{}
			}
		}

		// Assert that only one process ID is actively writing heartbeats
		assert.Len(t, activeProcesses, 1, "Only one process should be actively writing heartbeats")
	})
}

func TestCLIConfigPath(t *testing.T) {
	withTestContext(t, 10*time.Second, func(t *testing.T, tmpDir string) {
		// Define paths within the temporary directory
		customMultiYamlPath := filepath.Join(tmpDir, "custom_.devloop.yaml")
		cliTriggerFilePath := filepath.Join(tmpDir, "cli_trigger.txt")
		cliOutputFilePath := filepath.Join(tmpDir, "cli_output.txt")

		// Unique string to verify command execution
		cliUniqueString := "cli_command_executed_" + time.Now().Format("20060102150405")

		// Create custom_.devloop.yaml content
		customMultiYamlContent := fmt.Sprintf(`
rules:
  - name: "CLI Test Rule"
    watch:
      - action: include
        patterns:
          - "%s"
    commands:
      - "echo %s > %s"
`, filepath.Base(cliTriggerFilePath), cliUniqueString, cliOutputFilePath)

		// Write custom_.devloop.yaml
		err := os.WriteFile(customMultiYamlPath, []byte(customMultiYamlContent), 0644)
		assert.NoError(t, err)

		// 2. Run runApp in a goroutine
		orchestrator, err := NewOrchestrator(customMultiYamlPath, "")
		assert.NoError(t, err)
		assert.NotNil(t, orchestrator)

		// Start the orchestrator in a goroutine
		go func() {
			err := orchestrator.Start()
			assert.NoError(t, err)
		}()
		defer orchestrator.Stop()

		// Give the orchestrator some time to initialize
		time.Sleep(500 * time.Millisecond)

		// 3. Trigger and Verify
		// Create the trigger file
		err = os.WriteFile(cliTriggerFilePath, []byte("cli_trigger"), 0644)
		assert.NoError(t, err)

		// Poll for the output file to appear and contain the unique string
		timeout := time.After(5 * time.Second)
		for {
			select {
			case <-timeout:
				t.Fatal("Timeout waiting for CLI output file")
			default:
				content, readErr := os.ReadFile(cliOutputFilePath)
				if readErr == nil && strings.TrimSpace(string(content)) == cliUniqueString {
					return // Test success
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	})
}

func TestPrefixing(t *testing.T) {
	withTestContext(t, 10*time.Second, func(t *testing.T, tmpDir string) {
		// Define paths within the temporary directory
		multiYamlPath := filepath.Join(tmpDir, ".devloop.yaml")
		triggerFilePath := filepath.Join(tmpDir, "trigger.txt")

		// Create .devloop.yaml content
		multiYamlContent := fmt.Sprintf(`
settings:
  prefix_logs: true
  prefix_max_length: 10
rules:
  - name: "Prefix Test Rule"
    prefix: "prefix"
    watch:
      - action: include
        patterns:
          - "%s"
    commands:
      - "echo 'hello'"
`, filepath.Base(triggerFilePath))

		// Write .devloop.yaml
		err := os.WriteFile(multiYamlPath, []byte(multiYamlContent), 0644)
		assert.NoError(t, err)

		// 2. Run Orchestrator
		orchestrator, err := NewOrchestrator(multiYamlPath, "")
		assert.NoError(t, err)
		assert.NotNil(t, orchestrator)

		// Capture the output
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		defer func() {
			os.Stdout = oldStdout
		}()

		// Start the orchestrator in a goroutine
		go func() {
			err := orchestrator.Start()
			assert.NoError(t, err)
		}()
		defer orchestrator.Stop()

		// Give the watcher some time to initialize
		time.Sleep(500 * time.Millisecond)

		// 3. Trigger and Verify
		// Create the trigger file
		err = os.WriteFile(triggerFilePath, []byte("trigger"), 0644)
		assert.NoError(t, err)

		// Give the watcher some time to process the event
		time.Sleep(1 * time.Second)

		// Restore stdout
		w.Close()

		var buf bytes.Buffer
		io.Copy(&buf, r)
		output := buf.String()

		// Verify the output
		assert.Contains(t, output, "[prefix    ] hello")
	})
}
