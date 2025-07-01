package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGracefulShutdown(t *testing.T) {
	// 1. Setup Test Environment
	tmpDir, err := os.MkdirTemp("", "devloop_graceful_shutdown_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Change current working directory to tmpDir for relative paths to work
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	assert.NoError(t, os.Chdir(tmpDir))

	// Define paths within the temporary directory
	multiYamlPath := filepath.Join(tmpDir, "multi.yaml")
	triggerFilePath := filepath.Join(tmpDir, "trigger.txt")
	heartbeatFilePath := filepath.Join(tmpDir, "heartbeat.txt")

	// Create multi.yaml content with a long-running command
	multiYamlContent := fmt.Sprintf(`
rules:
  - name: "Heartbeat Rule"
    watch:
      - action: include
        patterns:
          - "%s"
    commands:
      - bash -c "while true; do echo \"heartbeat\" >> %s; sleep 0.1; done"
`, filepath.Base(triggerFilePath), heartbeatFilePath)

	// Write multi.yaml
	err = os.WriteFile(multiYamlPath, []byte(multiYamlContent), 0644)
	assert.NoError(t, err)

	// 2. Run devloop as a subprocess
	// Build the devloop executable into the temporary directory
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(tmpDir, "devloop"), ".")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	buildCmd.Dir = originalDir // Set the working directory for go build
	err = buildCmd.Run()
	assert.NoError(t, err, "Failed to build devloop executable")

	cmd := exec.Command(filepath.Join(tmpDir, "devloop"), "-c", "multi.yaml")
	cmd.Dir = tmpDir // Run the command in the temporary directory

	// Capture stdout and stderr
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Start the command
	err = cmd.Start()
	assert.NoError(t, err, "Failed to start devloop process: %v\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())

	// 3. Verify Child Process Activity
	// Give devloop some time to start
	time.Sleep(1 * time.Second)

	// Create the trigger file to start the heartbeat rule
	triggerFilePath = filepath.Join(tmpDir, "trigger.txt")
	err = os.WriteFile(triggerFilePath, []byte("trigger"), 0644)
	assert.NoError(t, err)

	// Poll for heartbeat file to be written to
	timeout := time.After(5 * time.Second)
	var initialHeartbeatContent []byte // Declare here
	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for heartbeat file to be written")
		default:
			content, readErr := os.ReadFile(heartbeatFilePath)
			if readErr == nil && len(content) > 0 {
				initialHeartbeatContent = content // Assign here
				goto EndHeartbeatCheck
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
EndHeartbeatCheck:

	// 4. Send SIGINT
	log.Println("Sending SIGINT to devloop process...")
	err = syscall.Kill(cmd.Process.Pid, syscall.SIGINT)
	assert.NoError(t, err, "Failed to send SIGINT to devloop process")

	// 5. Verify Termination
	// Wait for the devloop process to exit
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		assert.NoError(t, err, "devloop process did not exit cleanly")
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for devloop process to exit")
	}

	// Give a moment for the child process to fully terminate after devloop exits
	time.Sleep(500 * time.Millisecond)

	// Verify heartbeat file is no longer being written to
	finalHeartbeatContent, err := os.ReadFile(heartbeatFilePath)
	assert.NoError(t, err)
	assert.Equal(t, len(initialHeartbeatContent), len(finalHeartbeatContent), "Heartbeat file content should not have changed after shutdown")

	log.Printf("Devloop process stdout:\n%s", stdout.String())
	log.Printf("Devloop process stderr:\n%s", stderr.String())
}
