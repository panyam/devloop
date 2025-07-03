package agent

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/panyam/devloop/testhelpers"
	"github.com/stretchr/testify/assert"
)

// TestGracefulShutdown verifies that devloop can be gracefully shut down,
// properly stopping all running processes and cleaning up resources without
// leaving orphaned processes or hanging operations.
func TestGracefulShutdown(t *testing.T) {
	testhelpers.WithTestContext(t, 20*time.Second, func(t *testing.T, tmpDir string) {
		// Get the project root to build the binary correctly.
		originalDir, err := os.Getwd()
		assert.NoError(t, err)

		// Get the actual project root (parent of agent directory)
		projectRoot := filepath.Dir(originalDir)

		// 1. Build the devloop executable from the project root.
		buildCmd := exec.Command("go", "build", "-o", filepath.Join(tmpDir, "devloop"), ".")
		buildCmd.Dir = projectRoot // Ensure build runs in the project root.
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		err = buildCmd.Run()
		assert.NoError(t, err, "Failed to build devloop executable")

		// 2. Change into the temporary directory for the test execution.
		err = os.Chdir(tmpDir)
		assert.NoError(t, err)
		defer os.Chdir(originalDir) // Ensure we go back.

		// 3. Setup test files inside the temporary directory.
		multiYamlPath := filepath.Join(tmpDir, ".devloop.yaml")
		triggerFilePath := filepath.Join(tmpDir, "trigger.txt")
		heartbeatFilePath := filepath.Join(tmpDir, "heartbeat.txt")

		httpPort := "8888"
		grpcPort := "50051"
		multiYamlContent := fmt.Sprintf(`
rules:
  - name: "Heartbeat Rule"
    watch:
      - action: include
        patterns:
          - "trigger.txt"
    commands:
      - bash -c "while true; do echo \"heartbeat\" >> %s; sleep 0.1; done"
`, heartbeatFilePath)
		err = os.WriteFile(multiYamlPath, []byte(multiYamlContent), 0644)
		assert.NoError(t, err)

		// 4. Run devloop as a subprocess from within the temp directory.
		cmd := exec.Command("./devloop", "-c", ".devloop.yaml", "--mode", "standalone", "--http-port", httpPort, "--grpc-port", grpcPort, "-v")
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Start()
		assert.NoError(t, err, "Failed to start devloop process: %v\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())

		// Log the process ID for debugging
		t.Logf("Started devloop process with PID: %d", cmd.Process.Pid)

		// 5. Verify server readiness.
		client := &http.Client{Timeout: 1 * time.Second}
		serverURL := fmt.Sprintf("http://localhost:%s/", httpPort)
		assert.Eventually(t, func() bool {
			resp, err := client.Get(serverURL)
			if err != nil {
				return false
			}
			defer resp.Body.Close()
			return resp.StatusCode == http.StatusNotFound
		}, 10*time.Second, 500*time.Millisecond, "HTTP server did not become reachable")

		// Give the watcher time to initialize
		time.Sleep(2 * time.Second)

		// Verify we're in the right directory
		cwd, _ := os.Getwd()
		t.Logf("Current working directory: %s", cwd)
		t.Logf("Trigger file path: %s", triggerFilePath)

		// 6. Trigger the rule and verify activity.
		err = os.WriteFile(triggerFilePath, []byte("trigger"), 0644)
		assert.NoError(t, err)

		// Log that we created the file
		if _, err := os.Stat(triggerFilePath); err == nil {
			t.Logf("Successfully created trigger file at %s", triggerFilePath)
		}

		var initialHeartbeatContent []byte
		assert.Eventually(t, func() bool {
			content, readErr := os.ReadFile(heartbeatFilePath)
			if readErr == nil && len(content) > 0 {
				initialHeartbeatContent = content
				return true
			}
			return false
		}, 5*time.Second, 100*time.Millisecond, "Timeout waiting for heartbeat file")

		// 7. Send SIGINT and verify graceful shutdown.
		log.Println("Sending SIGINT to devloop process...")
		err = syscall.Kill(cmd.Process.Pid, syscall.SIGINT)
		assert.NoError(t, err, "Failed to send SIGINT to devloop process")

		waitDone := make(chan error, 1)
		go func() {
			waitDone <- cmd.Wait()
		}()
		select {
		case err := <-waitDone:
			assert.NoError(t, err, "devloop process did not exit cleanly")
		case <-time.After(10 * time.Second):
			t.Fatal("Timeout waiting for devloop process to exit")
		}

		time.Sleep(500 * time.Millisecond)

		finalHeartbeatContent, err := os.ReadFile(heartbeatFilePath)
		assert.NoError(t, err)
		assert.Equal(t, len(initialHeartbeatContent), len(finalHeartbeatContent), "Heartbeat file should not have changed after shutdown")

		_, err = client.Get(serverURL)
		assert.Error(t, err, "HTTP server should no longer be reachable")
		assert.Contains(t, err.Error(), "connection refused")

		t.Logf("Devloop process stdout:\n%s", stdout.String())
		t.Logf("Devloop process stderr:\n%s", stderr.String())
	})
}
