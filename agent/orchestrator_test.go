package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	withTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		// Test successful loading
		configPath := "../testdata/test_devloop.yaml"
		config, err := LoadConfig(configPath)
		assert.NoError(t, err)
		assert.NotNil(t, config)
		assert.Len(t, config.Rules, 2)

		assert.Equal(t, "Test Rule 1", config.Rules[0].Name)
		assert.Len(t, config.Rules[0].Watch, 1)
		assert.Equal(t, "include", config.Rules[0].Watch[0].Action)
		// Patterns should be resolved to absolute paths relative to config file
		absConfigPath, _ := filepath.Abs(configPath)
		expectedPath1 := filepath.Join(filepath.Dir(absConfigPath), "src/**/*.go")
		assert.Equal(t, []string{expectedPath1}, config.Rules[0].Watch[0].Patterns)
		assert.Equal(t, []string{"go build"}, config.Rules[0].Commands)

		assert.Equal(t, "Test Rule 2", config.Rules[1].Name)
		assert.Len(t, config.Rules[1].Watch, 1)
		assert.Equal(t, "include", config.Rules[1].Watch[0].Action)
		// Patterns should be resolved to absolute paths relative to config file
		expectedPath2 := filepath.Join(filepath.Dir(absConfigPath), "web/**/*.js")
		assert.Equal(t, []string{expectedPath2}, config.Rules[1].Watch[0].Patterns)
		assert.Equal(t, []string{"npm run build"}, config.Rules[1].Commands)

		// Test non-existent file
		_, err = LoadConfig("non_existent.yaml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "config file not found")

		// Test invalid YAML
		invalidConfigPath := "../testdata/invalid.yaml"
		_, err = LoadConfig(invalidConfigPath)
		assert.Error(t, err)
	})
}

func TestNewOrchestrator(t *testing.T) {
	withTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		// Test successful creation
		orchestrator, err := NewOrchestratorForTesting("../testdata/test_devloop.yaml")
		assert.NoError(t, err)
		assert.NotNil(t, orchestrator)
		assert.NotNil(t, orchestrator.GetConfig())

		// Test with non-existent config file
		_, err = NewOrchestratorForTesting("non_existent.yaml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")

		// Test with invalid config file
		_, err = NewOrchestratorForTesting("../testdata/invalid.yaml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})
}

func TestOrchestratorStartStop(t *testing.T) {
	withTestContext(t, 5*time.Second, func(t *testing.T, tmpDir string) {
		// Create a dummy .devloop.yaml in the temporary directory
		configPath := filepath.Join(tmpDir, ".devloop.yaml")
		dummyConfigContent := `
rules:
  - name: "Test Watch"
    watch:
      - action: include
        patterns:
          - "**/*"
    commands:
      - "echo 'File changed!'"
`
		err := os.WriteFile(configPath, []byte(dummyConfigContent), 0644)
		assert.NoError(t, err)

		orchestrator, err := NewOrchestratorForTesting(configPath)
		assert.NoError(t, err)
		assert.NotNil(t, orchestrator)

		// Start the orchestrator in a goroutine
		go func() {
			err := orchestrator.Start()
			assert.NoError(t, err)
		}()
		defer orchestrator.Stop()

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
	})
}

func TestRuleMatches(t *testing.T) {
	withTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		tests := []struct {
			name           string
			watchers       []*pb.RuleMatcher
			filePath       string
			expectedMatch  bool
			expectedAction string
		}{
			{
				name: "Simple include",
				watchers: []*pb.RuleMatcher{
					{Action: "include", Patterns: []string{"*.go"}},
				},
				filePath:       "main.go",
				expectedMatch:  true,
				expectedAction: "include",
			},
			{
				name: "Simple exclude",
				watchers: []*pb.RuleMatcher{
					{Action: "exclude", Patterns: []string{"*.go"}},
				},
				filePath:       "main.go",
				expectedMatch:  true,
				expectedAction: "exclude",
			},
			{
				name: "Include then exclude (include wins)",
				watchers: []*pb.RuleMatcher{
					{Action: "include", Patterns: []string{"*.go"}},
					{Action: "exclude", Patterns: []string{"main.go"}},
				},
				filePath:       "main.go",
				expectedMatch:  true,
				expectedAction: "include",
			},
			{
				name: "Exclude then include (exclude wins)",
				watchers: []*pb.RuleMatcher{
					{Action: "exclude", Patterns: []string{"*.go"}},
					{Action: "include", Patterns: []string{"main.go"}},
				},
				filePath:       "main.go",
				expectedMatch:  true,
				expectedAction: "exclude",
			},
			{
				name: "No match",
				watchers: []*pb.RuleMatcher{
					{Action: "include", Patterns: []string{"*.js"}},
				},
				filePath:      "main.go",
				expectedMatch: false,
			},
			{
				name: "Complex rule: include specific, exclude folder, include general",
				watchers: []*pb.RuleMatcher{
					{Action: "include", Patterns: []string{"vendor/specific/file.go"}},
					{Action: "exclude", Patterns: []string{"vendor/**"}},
					{Action: "include", Patterns: []string{"**/*.go"}},
				},
				filePath:       "vendor/specific/file.go",
				expectedMatch:  true,
				expectedAction: "include",
			},
			{
				name: "Complex rule: file in excluded folder",
				watchers: []*pb.RuleMatcher{
					{Action: "include", Patterns: []string{"vendor/specific/file.go"}},
					{Action: "exclude", Patterns: []string{"vendor/**"}},
					{Action: "include", Patterns: []string{"**/*.go"}},
				},
				filePath:       "vendor/other/file.go",
				expectedMatch:  true,
				expectedAction: "exclude",
			},
			{
				name: "Complex rule: general go file",
				watchers: []*pb.RuleMatcher{
					{Action: "include", Patterns: []string{"vendor/specific/file.go"}},
					{Action: "exclude", Patterns: []string{"vendor/**"}},
					{Action: "include", Patterns: []string{"**/*.go"}},
				},
				filePath:       "src/main.go",
				expectedMatch:  true,
				expectedAction: "include",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				rule := &pb.Rule{
					Watch: tt.watchers,
				}
				matcher := RuleMatches(rule, tt.filePath)
				if tt.expectedMatch {
					assert.NotNil(t, matcher)
					assert.Equal(t, tt.expectedAction, matcher.Action)
				} else {
					assert.Nil(t, matcher)
				}
			})
		}
	})
}
