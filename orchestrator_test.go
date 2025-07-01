package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	// Test successful loading
	configPath := "./testdata/test_multi.yaml"
	config, err := LoadConfig(configPath)
	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Len(t, config.Rules, 2)

	assert.Equal(t, "Test Rule 1", config.Rules[0].Name)
	assert.Equal(t, []string{"src/**/*.go"}, config.Rules[0].Watch)
	assert.Equal(t, []string{"go build"}, config.Rules[0].Commands)

	assert.Equal(t, "Test Rule 2", config.Rules[1].Name)
	assert.Equal(t, []string{"web/**/*.js"}, config.Rules[1].Watch)
	assert.Equal(t, []string{"npm run build"}, config.Rules[1].Commands)

	// Test non-existent file
	_, err = LoadConfig("non_existent.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config file not found")

	// Test invalid YAML
	invalidConfigPath := "./testdata/invalid.yaml"
	_, err = LoadConfig(invalidConfigPath)
	assert.Error(t, err)
}

func TestNewOrchestrator(t *testing.T) {
	// Test successful creation
	orchestrator, err := NewOrchestrator("./testdata/test_multi.yaml")
	assert.NoError(t, err)
	assert.NotNil(t, orchestrator)
	assert.NotNil(t, orchestrator.Config)
	assert.NotNil(t, orchestrator.Watcher)

	// Test with non-existent config file
	_, err = NewOrchestrator("non_existent.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")

	// Test with invalid config file
	_, err = NewOrchestrator("./testdata/invalid.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

func TestOrchestratorStartStop(t *testing.T) {
	// Create a temporary directory for testing file watches
	tmpDir, err := os.MkdirTemp("", "devloop_watch_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a dummy multi.yaml in the temporary directory
	configPath := filepath.Join(tmpDir, "multi.yaml")
	dummyConfigContent := `
rules:
  - name: "Test Watch"
    watch:
      - "**/*"
    commands:
      - "echo 'File changed!'"
`
	err = os.WriteFile(configPath, []byte(dummyConfigContent), 0644)
	assert.NoError(t, err)

	// Change current working directory to tmpDir for relative paths to work
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	assert.NoError(t, os.Chdir(tmpDir))

	orchestrator, err := NewOrchestrator("multi.yaml")
	assert.NoError(t, err)
	assert.NotNil(t, orchestrator)

	// Start the orchestrator in a goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		err := orchestrator.Start()
		assert.NoError(t, err)
	}()

	// Give the watcher some time to initialize
	time.Sleep(100 * time.Millisecond)

	// Test file creation
	filePath := filepath.Join(tmpDir, "test_file.txt")
	err = os.WriteFile(filePath, []byte("hello"), 0644)
	assert.NoError(t, err)

	// Give the watcher some time to process the event
	time.Sleep(100 * time.Millisecond)

	// Test directory creation and file inside it
	newDirPath := filepath.Join(tmpDir, "new_dir")
	err = os.Mkdir(newDirPath, 0755)
	assert.NoError(t, err)

	newFilePath := filepath.Join(newDirPath, "new_file.txt")
	err = os.WriteFile(newFilePath, []byte("world"), 0644)
	assert.NoError(t, err)

	// Give the watcher some time to process the event
	time.Sleep(100 * time.Millisecond)

	// Stop the orchestrator
	err = orchestrator.Stop()
	assert.NoError(t, err)

	// Wait for the orchestrator's Start goroutine to finish
	<-done
}

func TestRuleMatches(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		filePath string
		expected bool
	}{
		{
			name:     "Exact match",
			patterns: []string{"main.go"},
			filePath: "main.go",
			expected: true,
		},
		{
			name:     "Wildcard match",
			patterns: []string{"*.go"},
			filePath: "main.go",
			expected: true,
		},
		{
			name:     "Recursive wildcard match",
			patterns: []string{"**/*.go"},
			filePath: "src/cmd/server/main.go",
			expected: true,
		},
		{
			name:     "No match",
			patterns: []string{"*.js"},
			filePath: "main.go",
			expected: false,
		},
		{
			name:     "Multiple patterns - one matches",
			patterns: []string{"*.js", "**/*.css"},
			filePath: "web/style.css",
			expected: true,
		},
		{
			name:     "Multiple patterns - none match",
			patterns: []string{"*.js", "*.css"},
			filePath: "index.html",
			expected: false,
		},
		{
			name:     "Directory match (should not match file)",
			patterns: []string{"src/"},
			filePath: "src/main.go",
			expected: false,
		},
		{
			name:     "Directory match with recursive wildcard",
			patterns: []string{"src/**"},
			filePath: "src/main.go",
			expected: true,
		},
		{
			name:     "Directory match with recursive wildcard for directory",
			patterns: []string{"src/**"},
			filePath: "src/sub/",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := Rule{
				Watch: tt.patterns,
			}
			assert.Equal(t, tt.expected, rule.Matches(tt.filePath))
		})
	}
}
