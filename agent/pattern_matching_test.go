package agent

import (
	"testing"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"github.com/stretchr/testify/assert"
)

// TestDirectoryWatchingLogic tests what directories should be watched
// Directory watching should only consider INCLUDE patterns
func TestDirectoryWatchingLogic(t *testing.T) {
	tests := []struct {
		name        string
		patterns    []*pb.RuleMatcher
		directory   string
		shouldWatch bool
		description string
	}{
		{
			name: "Include pattern matches directory",
			patterns: []*pb.RuleMatcher{
				{Action: "include", Patterns: []string{"lib/**/*.go"}},
			},
			directory:   "lib",
			shouldWatch: true,
			description: "lib/ should be watched because lib/**/*.go could match files in it",
		},
		{
			name: "Exclude pattern alone should not prevent watching",
			patterns: []*pb.RuleMatcher{
				{Action: "exclude", Patterns: []string{"**/*.log"}},
			},
			directory:   "lib",
			shouldWatch: false,
			description: "lib/ should NOT be watched - only exclude patterns, no includes",
		},
		{
			name: "WeeWar scenario - include should win for directory watching",
			patterns: []*pb.RuleMatcher{
				{Action: "include", Patterns: []string{"web/server/*.go"}},
				{Action: "exclude", Patterns: []string{"**/*.log", "web/**/*", "web"}},
				{Action: "include", Patterns: []string{"lib/**/*.go", "cmd/**/*.go"}},
			},
			directory:   "lib",
			shouldWatch: true,
			description: "lib/ should be watched because lib/**/*.go (include) could match files",
		},
		{
			name: "WeeWar scenario - web/server should be watched",
			patterns: []*pb.RuleMatcher{
				{Action: "include", Patterns: []string{"web/server/*.go"}},
				{Action: "exclude", Patterns: []string{"**/*.log", "web/**/*", "web"}},
				{Action: "include", Patterns: []string{"lib/**/*.go", "cmd/**/*.go"}},
			},
			directory:   "web/server",
			shouldWatch: true,
			description: "web/server/ should be watched because web/server/*.go (include) could match files",
		},
		{
			name: "No matching include patterns",
			patterns: []*pb.RuleMatcher{
				{Action: "include", Patterns: []string{"src/**/*.go"}},
				{Action: "exclude", Patterns: []string{"**/*.log"}},
			},
			directory:   "lib",
			shouldWatch: false,
			description: "lib/ should NOT be watched - no include patterns could match files in it",
		},
		{
			name: "Multiple include patterns, one matches",
			patterns: []*pb.RuleMatcher{
				{Action: "include", Patterns: []string{"src/**/*.go", "lib/**/*.go", "test/**/*.go"}},
			},
			directory:   "lib",
			shouldWatch: true,
			description: "lib/ should be watched because lib/**/*.go could match files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock rule with the test patterns
			rule := &pb.Rule{
				Name:    "test-rule",
				Watch:   tt.patterns,
				WorkDir: "/test/project",
			}

			// Create watcher
			watcher := &Watcher{
				rule:       rule,
				configPath: "/test/project/.devloop.yaml",
			}

			// Test directory watching decision with absolute path (like real usage)
			absoluteDir := "/test/project/" + tt.directory
			result := watcher.shouldWatchDirectory(absoluteDir)
			assert.Equal(t, tt.shouldWatch, result, tt.description)
		})
	}
}

// TestFileFilteringLogic tests what files should trigger rules (FIFO logic)
// File filtering should apply FIFO - first matching pattern wins
func TestFileFilteringLogic(t *testing.T) {
	tests := []struct {
		name         string
		patterns     []*pb.RuleMatcher
		filePath     string
		shouldTrigger bool
		description  string
	}{
		{
			name: "First include pattern wins",
			patterns: []*pb.RuleMatcher{
				{Action: "include", Patterns: []string{"lib/**/*.go"}},
				{Action: "exclude", Patterns: []string{"**/*.log"}},
			},
			filePath:     "lib/utils.go",
			shouldTrigger: true,
			description:  "lib/utils.go should trigger - first pattern lib/**/*.go (include) matches",
		},
		{
			name: "First exclude pattern wins",
			patterns: []*pb.RuleMatcher{
				{Action: "exclude", Patterns: []string{"**/*.log"}},
				{Action: "include", Patterns: []string{"lib/**/*"}},
			},
			filePath:     "lib/debug.log",
			shouldTrigger: false,
			description:  "lib/debug.log should NOT trigger - first pattern **/*.log (exclude) matches",
		},
		{
			name: "WeeWar scenario - web/server/*.go wins over web/**/*",
			patterns: []*pb.RuleMatcher{
				{Action: "include", Patterns: []string{"web/server/*.go"}},
				{Action: "exclude", Patterns: []string{"**/*.log", "web/**/*", "web"}},
				{Action: "include", Patterns: []string{"lib/**/*.go"}},
			},
			filePath:     "web/server/main.go",
			shouldTrigger: true,
			description:  "web/server/main.go should trigger - first pattern web/server/*.go (include) matches",
		},
		{
			name: "WeeWar scenario - lib files should trigger",
			patterns: []*pb.RuleMatcher{
				{Action: "include", Patterns: []string{"web/server/*.go"}},
				{Action: "exclude", Patterns: []string{"**/*.log", "web/**/*", "web"}},
				{Action: "include", Patterns: []string{"lib/**/*.go"}},
			},
			filePath:     "lib/utils.go",
			shouldTrigger: true,
			description:  "lib/utils.go should trigger - no earlier patterns match, lib/**/*.go (include) matches",
		},
		{
			name: "WeeWar scenario - log files should be excluded",
			patterns: []*pb.RuleMatcher{
				{Action: "include", Patterns: []string{"web/server/*.go"}},
				{Action: "exclude", Patterns: []string{"**/*.log", "web/**/*", "web"}},
				{Action: "include", Patterns: []string{"lib/**/*.go"}},
			},
			filePath:     "lib/debug.log",
			shouldTrigger: false,
			description:  "lib/debug.log should NOT trigger - **/*.log (exclude) matches first",
		},
		{
			name: "No patterns match",
			patterns: []*pb.RuleMatcher{
				{Action: "include", Patterns: []string{"src/**/*.go"}},
			},
			filePath:     "lib/utils.go",
			shouldTrigger: false,
			description:  "lib/utils.go should NOT trigger - no patterns match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock rule with the test patterns
			rule := &pb.Rule{
				Name:    "test-rule",
				Watch:   tt.patterns,
				WorkDir: "/test/project",
			}

			// Test file filtering decision using RuleMatches (which implements FIFO)
			// Use absolute path like real usage
			absoluteFilePath := "/test/project/" + tt.filePath
			matcher := RuleMatches(rule, absoluteFilePath, "/test/project/.devloop.yaml")
			result := matcher != nil && matcher.Action == "include"
			assert.Equal(t, tt.shouldTrigger, result, tt.description)
		})
	}
}

// TestDirectoryVsFileLogic tests the integration of directory watching and file filtering
func TestDirectoryVsFileLogic(t *testing.T) {
	// WeeWar reproduction test case
	patterns := []*pb.RuleMatcher{
		{Action: "include", Patterns: []string{"web/server/*.go"}},
		{Action: "exclude", Patterns: []string{"**/*.log", "web/**/*", "web"}},
		{Action: "include", Patterns: []string{"lib/**/*.go", "cmd/**/*.go", "services/**/*.go"}},
	}

	rule := &pb.Rule{
		Name:    "betests",
		Watch:   patterns,
		WorkDir: "/project/root",
	}

	watcher := &Watcher{
		rule:       rule,
		configPath: "/project/root/.devloop.yaml",
	}

	t.Run("DirectoryWatchingDecisions", func(t *testing.T) {
		// These directories should be watched (have include patterns)
		assert.True(t, watcher.shouldWatchDirectory("/project/root/lib"), 
			"lib/ should be watched - lib/**/*.go include pattern")
		assert.True(t, watcher.shouldWatchDirectory("/project/root/cmd"), 
			"cmd/ should be watched - cmd/**/*.go include pattern")
		assert.True(t, watcher.shouldWatchDirectory("/project/root/web/server"), 
			"web/server/ should be watched - web/server/*.go include pattern")
		
		// These directories should NOT be watched (no relevant include patterns)
		assert.False(t, watcher.shouldWatchDirectory("/project/root/docs"), 
			"docs/ should NOT be watched - no include patterns match")
		assert.False(t, watcher.shouldWatchDirectory("/project/root/tmp"), 
			"tmp/ should NOT be watched - no include patterns match")
	})

	t.Run("FileFilteringDecisions", func(t *testing.T) {
		// Helper function to check if file should trigger rule
		shouldTrigger := func(filePath string) bool {
			matcher := RuleMatches(rule, filePath, "/project/root/.devloop.yaml")
			return matcher != nil && matcher.Action == "include"
		}

		// These files should trigger (include patterns win via FIFO)
		assert.True(t, shouldTrigger("web/server/main.go"), 
			"web/server/main.go should trigger - web/server/*.go include pattern wins")
		assert.True(t, shouldTrigger("lib/utils.go"), 
			"lib/utils.go should trigger - lib/**/*.go include pattern matches")
		assert.True(t, shouldTrigger("cmd/server/main.go"), 
			"cmd/server/main.go should trigger - cmd/**/*.go include pattern matches")

		// These files should NOT trigger (exclude patterns win via FIFO)  
		assert.False(t, shouldTrigger("lib/debug.log"), 
			"lib/debug.log should NOT trigger - **/*.log exclude pattern wins")
		assert.False(t, shouldTrigger("web/dist/bundle.js"), 
			"web/dist/bundle.js should NOT trigger - web/**/* exclude pattern wins")
		assert.False(t, shouldTrigger("services/debug.log"), 
			"services/debug.log should NOT trigger - **/*.log exclude pattern wins")
			
		// These files should NOT trigger (no patterns match)
		assert.False(t, shouldTrigger("docs/readme.md"), 
			"docs/readme.md should NOT trigger - no patterns match")
	})
}

// TestPatternCouldMatchInDirectory tests the core pattern matching helper
func TestPatternCouldMatchInDirectory(t *testing.T) {
	watcher := &Watcher{
		rule:       &pb.Rule{Name: "test"},
		configPath: "/test/.devloop.yaml",
	}

	tests := []struct {
		pattern   string
		directory string
		expected  bool
		reason    string
	}{
		{"lib/**/*.go", "lib", true, "lib/**/*.go should match files in lib/"},
		{"lib/**/*.go", "lib/subdir", true, "lib/**/*.go should match files in lib/subdir/"},
		{"lib/**/*.go", "cmd", false, "lib/**/*.go should NOT match files in cmd/"},
		{"**/*.log", "lib", true, "**/*.log could match lib/test.log"},
		{"**/*.log", "any/dir", true, "**/*.log could match files in any directory"},
		{"web/server/*.go", "web/server", true, "web/server/*.go should match files in web/server/"},
		{"web/server/*.go", "web", false, "web/server/*.go should NOT match files directly in web/"},
		{"web/server/*.go", "web/client", false, "web/server/*.go should NOT match files in web/client/"},
		{"specific.txt", "dir", false, "specific.txt should NOT match files in any directory"},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_vs_"+tt.directory, func(t *testing.T) {
			result := watcher.patternCouldMatchInDirectory(tt.pattern, tt.directory)
			assert.Equal(t, tt.expected, result, tt.reason)
		})
	}
}
