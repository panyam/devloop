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
		// Patterns should be preserved as relative paths
		assert.Equal(t, []string{"src/**/*.go"}, config.Rules[0].Watch[0].Patterns)
		assert.Equal(t, []string{"go build"}, config.Rules[0].Commands)

		assert.Equal(t, "Test Rule 2", config.Rules[1].Name)
		assert.Len(t, config.Rules[1].Watch, 1)
		assert.Equal(t, "include", config.Rules[1].Watch[0].Action)
		// Patterns should be preserved as relative paths
		assert.Equal(t, []string{"web/**/*.js"}, config.Rules[1].Watch[0].Patterns)
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
				// Create a config path in the tmpDir for proper pattern resolution
				configPath := filepath.Join(tmpDir, ".devloop.yaml")

				// Create absolute path for the test file
				absFilePath := filepath.Join(tmpDir, tt.filePath)

				matcher := RuleMatches(rule, absFilePath, configPath)
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

func TestWorkdirRelativePatterns(t *testing.T) {
	withTestContext(t, 5*time.Second, func(t *testing.T, tmpDir string) {
		// Create test directory structure
		backendDir := filepath.Join(tmpDir, "backend")
		err := os.MkdirAll(filepath.Join(backendDir, "src"), 0755)
		assert.NoError(t, err)

		frontendDir := filepath.Join(tmpDir, "frontend")
		err = os.MkdirAll(filepath.Join(frontendDir, "src"), 0755)
		assert.NoError(t, err)

		// Create test config with workdir-relative patterns
		configContent := `
rules:
  - name: "Backend Rule"
    workdir: "./backend"
    watch:
      - action: include
        patterns:
          - "src/**/*.go"
    commands:
      - "echo 'Backend file changed' >> output.txt"
  - name: "Frontend Rule"
    workdir: "./frontend"
    watch:
      - action: include
        patterns:
          - "src/**/*.js"
    commands:
      - "echo 'Frontend file changed' >> output.txt"
`
		configPath := filepath.Join(tmpDir, ".devloop.yaml")
		err = os.WriteFile(configPath, []byte(configContent), 0644)
		assert.NoError(t, err)

		// Load config and test pattern resolution
		config, err := LoadConfig(configPath)
		assert.NoError(t, err)
		assert.Len(t, config.Rules, 2)

		// Test that patterns are preserved as relative
		assert.Equal(t, "src/**/*.go", config.Rules[0].Watch[0].Patterns[0])
		assert.Equal(t, "src/**/*.js", config.Rules[1].Watch[0].Patterns[0])

		// Test RuleMatches with workdir-relative patterns
		backendRule := config.Rules[0]
		frontendRule := config.Rules[1]

		// Test backend file matching
		backendFile := filepath.Join(backendDir, "src", "main.go")
		matcher := RuleMatches(backendRule, backendFile, configPath)
		assert.NotNil(t, matcher)
		assert.Equal(t, "include", matcher.Action)

		// Test frontend file matching
		frontendFile := filepath.Join(frontendDir, "src", "app.js")
		matcher = RuleMatches(frontendRule, frontendFile, configPath)
		assert.NotNil(t, matcher)
		assert.Equal(t, "include", matcher.Action)

		// Test cross-matching (backend rule should NOT match frontend files)
		matcher = RuleMatches(backendRule, frontendFile, configPath)
		assert.Nil(t, matcher)

		// Test cross-matching (frontend rule should NOT match backend files)
		matcher = RuleMatches(frontendRule, backendFile, configPath)
		assert.Nil(t, matcher)

		// Test files in wrong location (should not match)
		wrongFile := filepath.Join(tmpDir, "src", "wrong.go")
		matcher = RuleMatches(backendRule, wrongFile, configPath)
		assert.Nil(t, matcher)
	})
}

func TestCycleDetection(t *testing.T) {
	withTestContext(t, 5*time.Second, func(t *testing.T, tmpDir string) {
		// Create test directory structure
		workDir := filepath.Join(tmpDir, "test_work")
		err := os.MkdirAll(workDir, 0755)
		assert.NoError(t, err)

		tests := []struct {
			name           string
			config         *pb.Config
			expectWarnings bool
			expectErrors   bool
		}{
			{
				name: "Self-referential rule with cycle detection enabled",
				config: &pb.Config{
					Settings: &pb.Settings{
						CycleDetection: &pb.CycleDetectionSettings{
							Enabled:          true,
							StaticValidation: true,
						},
					},
					Rules: []*pb.Rule{
						{
							Name:    "Self-Ref Rule",
							WorkDir: workDir,
							Watch: []*pb.RuleMatcher{
								{
									Action:   "include",
									Patterns: []string{"**/*.log"},
								},
							},
							Commands: []string{"echo 'test' >> test.log"},
						},
					},
				},
				expectWarnings: true,
				expectErrors:   false,
			},
			{
				name: "Self-referential rule with cycle detection disabled globally",
				config: &pb.Config{
					Settings: &pb.Settings{
						CycleDetection: &pb.CycleDetectionSettings{
							Enabled:          false,
							StaticValidation: true,
						},
					},
					Rules: []*pb.Rule{
						{
							Name:    "Self-Ref Rule",
							WorkDir: workDir,
							Watch: []*pb.RuleMatcher{
								{
									Action:   "include",
									Patterns: []string{"**/*.log"},
								},
							},
							Commands: []string{"echo 'test' >> test.log"},
						},
					},
				},
				expectWarnings: false,
				expectErrors:   false,
			},
			{
				name: "Self-referential rule with per-rule cycle protection disabled",
				config: &pb.Config{
					Settings: &pb.Settings{
						CycleDetection: &pb.CycleDetectionSettings{
							Enabled:          true,
							StaticValidation: true,
						},
					},
					Rules: []*pb.Rule{
						{
							Name:            "Self-Ref Rule",
							WorkDir:         workDir,
							CycleProtection: boolPtr(false),
							Watch: []*pb.RuleMatcher{
								{
									Action:   "include",
									Patterns: []string{"**/*.log"},
								},
							},
							Commands: []string{"echo 'test' >> test.log"},
						},
					},
				},
				expectWarnings: false,
				expectErrors:   false,
			},
			{
				name: "Non-self-referential rule",
				config: &pb.Config{
					Settings: &pb.Settings{
						CycleDetection: &pb.CycleDetectionSettings{
							Enabled:          true,
							StaticValidation: true,
						},
					},
					Rules: []*pb.Rule{
						{
							Name:    "Safe Rule",
							WorkDir: workDir,
							Watch: []*pb.RuleMatcher{
								{
									Action:   "include",
									Patterns: []string{"../other/**/*.go"},
								},
							},
							Commands: []string{"echo 'test' >> test.log"},
						},
					},
				},
				expectWarnings: false,
				expectErrors:   false,
			},
			{
				name: "Default configuration (no cycle detection settings)",
				config: &pb.Config{
					Settings: &pb.Settings{}, // No cycle detection settings
					Rules: []*pb.Rule{
						{
							Name:    "Default Rule",
							WorkDir: workDir,
							Watch: []*pb.RuleMatcher{
								{
									Action:   "include",
									Patterns: []string{"**/*.log"},
								},
							},
							Commands: []string{"echo 'test' >> test.log"},
						},
					},
				},
				expectWarnings: true, // Should warn with default settings
				expectErrors:   false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Create temporary config file
				configPath := filepath.Join(tmpDir, "test_config.yaml")

				// Create orchestrator with test config
				orchestrator := &Orchestrator{
					ConfigPath: configPath,
					Config:     tt.config,
				}

				// Test cycle detection
				err := orchestrator.ValidateConfig()
				if tt.expectErrors {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}

				// Test individual helper methods
				settings := orchestrator.getCycleDetectionSettings()
				assert.NotNil(t, settings)

				if tt.config.Settings.CycleDetection != nil {
					assert.Equal(t, tt.config.Settings.CycleDetection.Enabled, orchestrator.isCycleDetectionEnabled())
					// Note: Static validation removed - only test dynamic protection
					assert.Equal(t, tt.config.Settings.CycleDetection.DynamicProtection, orchestrator.isDynamicProtectionEnabled())
				}

				// Test per-rule cycle protection
				if len(tt.config.Rules) > 0 {
					rule := tt.config.Rules[0]
					expected := true // default
					if rule.CycleProtection != nil {
						expected = *rule.CycleProtection
					} else if tt.config.Settings.CycleDetection != nil {
						expected = tt.config.Settings.CycleDetection.Enabled
					}
					assert.Equal(t, expected, orchestrator.isRuleCycleProtectionEnabled(rule))
				}
			})
		}
	})
}

// Helper function to create bool pointer
func boolPtr(b bool) *bool {
	return &b
}
