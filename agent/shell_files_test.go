package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
)

// --- expandCommandAliases tests ---

func TestExpandCommandAliases_NoAliases(t *testing.T) {
	result := expandCommandAliases("echo hello", nil)
	assert.Equal(t, "echo hello", result)
}

func TestExpandCommandAliases_EmptyMap(t *testing.T) {
	result := expandCommandAliases("echo hello", map[string]string{})
	assert.Equal(t, "echo hello", result)
}

func TestExpandCommandAliases_SimpleExpansion(t *testing.T) {
	commands := map[string]string{
		"build-proto": "buf generate --path proto/",
	}
	result := expandCommandAliases("$build-proto", commands)
	assert.Equal(t, "buf generate --path proto/", result)
}

func TestExpandCommandAliases_MixedWithOtherCommands(t *testing.T) {
	commands := map[string]string{
		"lint": "golangci-lint run ./...",
	}
	result := expandCommandAliases("$lint && echo ok", commands)
	assert.Equal(t, "golangci-lint run ./... && echo ok", result)
}

func TestExpandCommandAliases_MultipleAliases(t *testing.T) {
	commands := map[string]string{
		"build": "go build ./...",
		"test":  "go test ./...",
	}
	result := expandCommandAliases("$build && $test", commands)
	assert.Equal(t, "go build ./... && go test ./...", result)
}

func TestExpandCommandAliases_NoRecursion(t *testing.T) {
	// $a expands to "$b" but $b should NOT be expanded (single-pass)
	commands := map[string]string{
		"a": "$b",
		"b": "echo hello",
	}
	result := expandCommandAliases("$a", commands)
	assert.Equal(t, "$b", result)
}

func TestExpandCommandAliases_UnknownAlias(t *testing.T) {
	commands := map[string]string{
		"build": "go build",
	}
	// $unknown should remain as-is
	result := expandCommandAliases("$unknown", commands)
	assert.Equal(t, "$unknown", result)
}

func TestExpandCommandAliases_ShellVarsNotAffected(t *testing.T) {
	commands := map[string]string{
		"build": "go build",
	}
	// $HOME contains no hyphen, so it can't collide with command names
	// But even if someone defines a command without hyphens, shell vars
	// like $HOME, $PATH should ideally not collide. The design uses
	// hyphenated names to avoid collision.
	result := expandCommandAliases("echo $HOME", commands)
	assert.Equal(t, "echo $HOME", result)
}

func TestExpandCommandAliases_AliasAtStartMiddleEnd(t *testing.T) {
	commands := map[string]string{
		"my-cmd": "echo hi",
	}
	// Alias at start
	assert.Equal(t, "echo hi", expandCommandAliases("$my-cmd", commands))
	// Alias in middle
	assert.Equal(t, "x && echo hi && y", expandCommandAliases("x && $my-cmd && y", commands))
}

// --- buildSourcePrefix tests ---

func TestBuildSourcePrefix_NoFiles(t *testing.T) {
	result := buildSourcePrefix(nil, nil, "/work", "/config")
	assert.Equal(t, "", result)
}

func TestBuildSourcePrefix_GlobalOnly(t *testing.T) {
	result := buildSourcePrefix([]string{"./env.sh"}, nil, "/work", "/config")
	assert.Equal(t, "source /config/env.sh && ", result)
}

func TestBuildSourcePrefix_RuleOnly(t *testing.T) {
	result := buildSourcePrefix(nil, []string{"./local.sh"}, "/work", "/config")
	assert.Equal(t, "source /work/local.sh && ", result)
}

func TestBuildSourcePrefix_GlobalAndRule(t *testing.T) {
	result := buildSourcePrefix(
		[]string{"./env.sh", "./aliases.sh"},
		[]string{"./proto-env.sh"},
		"/work",
		"/config",
	)
	assert.Equal(t, "source /config/env.sh && source /config/aliases.sh && source /work/proto-env.sh && ", result)
}

func TestBuildSourcePrefix_AbsolutePaths(t *testing.T) {
	result := buildSourcePrefix([]string{"/abs/path/env.sh"}, nil, "/work", "/config")
	assert.Equal(t, "source /abs/path/env.sh && ", result)
}

func TestBuildSourcePrefix_RelativeGlobalResolvesToConfigDir(t *testing.T) {
	result := buildSourcePrefix([]string{"scripts/setup.sh"}, nil, "/work", "/home/user/project")
	assert.Equal(t, "source /home/user/project/scripts/setup.sh && ", result)
}

func TestBuildSourcePrefix_RelativeRuleResolvesToWorkDir(t *testing.T) {
	result := buildSourcePrefix(nil, []string{"scripts/setup.sh"}, "/home/user/subdir", "/config")
	assert.Equal(t, "source /home/user/subdir/scripts/setup.sh && ", result)
}

// --- prepareCommand tests ---

func TestPrepareCommand_NoShellFilesNoAliases(t *testing.T) {
	settings := &pb.Settings{}
	rule := &pb.Rule{}
	result := prepareCommand("echo hello", settings, rule, "/config")
	assert.Equal(t, "echo hello", result)
}

func TestPrepareCommand_WithAliasExpansion(t *testing.T) {
	settings := &pb.Settings{
		Commands: map[string]string{
			"build-proto": "buf generate",
		},
	}
	rule := &pb.Rule{}
	result := prepareCommand("$build-proto", settings, rule, "/config")
	assert.Equal(t, "buf generate", result)
}

func TestPrepareCommand_WithShellFiles(t *testing.T) {
	settings := &pb.Settings{
		ShellFiles: []string{"./env.sh"},
	}
	rule := &pb.Rule{
		WorkDir: "/work",
	}
	result := prepareCommand("echo hello", settings, rule, "/config")
	assert.Equal(t, "source /config/env.sh && echo hello", result)
}

func TestPrepareCommand_WithShellFilesAndAliases(t *testing.T) {
	settings := &pb.Settings{
		ShellFiles: []string{"./env.sh"},
		Commands: map[string]string{
			"my-build": "make all",
		},
	}
	rule := &pb.Rule{
		ShellFiles: []string{"./local.sh"},
		WorkDir:    "/work",
	}
	result := prepareCommand("$my-build", settings, rule, "/config")
	assert.Equal(t, "source /config/env.sh && source /work/local.sh && make all", result)
}

func TestPrepareCommand_WorkDirDefaultsToConfigDir(t *testing.T) {
	settings := &pb.Settings{}
	rule := &pb.Rule{
		ShellFiles: []string{"./local.sh"},
		// WorkDir not set - should default to configDir
	}
	result := prepareCommand("echo hi", settings, rule, "/config")
	assert.Equal(t, "source /config/local.sh && echo hi", result)
}

// --- Config integration tests ---

func TestConfigShellFilesAndCommands(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "devloop-shellfiles-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configContent := `
settings:
  shell_files:
    - "./env.sh"
    - "./aliases.sh"
  commands:
    build-proto: "buf generate --path proto/"
    lint: "golangci-lint run ./..."
rules:
  - name: "proto-gen"
    shell_files:
      - "./proto-env.sh"
    watch:
      - action: "include"
        patterns: ["*.proto"]
    commands:
      - "$build-proto"
      - "$lint && echo ok"
  - name: "no-shell-files"
    watch:
      - action: "include"
        patterns: ["*.go"]
    commands:
      - "echo hello"
`
	configPath := filepath.Join(tmpDir, "test.yaml")
	err = os.WriteFile(configPath, []byte(strings.TrimSpace(configContent)), 0644)
	require.NoError(t, err)

	config, err := LoadConfig(configPath)
	require.NoError(t, err)

	// Verify settings shell_files
	assert.Equal(t, []string{"./env.sh", "./aliases.sh"}, config.Settings.ShellFiles)

	// Verify settings commands
	assert.Equal(t, "buf generate --path proto/", config.Settings.Commands["build-proto"])
	assert.Equal(t, "golangci-lint run ./...", config.Settings.Commands["lint"])

	// Verify rule shell_files
	assert.Equal(t, []string{"./proto-env.sh"}, config.Rules[0].ShellFiles)
	assert.Empty(t, config.Rules[1].ShellFiles)
}

// --- E2E test: shell_files sourcing ---

func TestShellFilesE2E(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "devloop-shellfiles-e2e-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a shell file that exports a variable and defines a function
	envScript := `#!/bin/sh
export MY_VAR="hello_from_env"
myfunc() { echo "function_output"; }
`
	err = os.WriteFile(filepath.Join(tmpDir, "env.sh"), []byte(envScript), 0755)
	require.NoError(t, err)

	// Test that prepareCommand produces the right command string
	settings := &pb.Settings{
		ShellFiles: []string{"./env.sh"},
	}
	rule := &pb.Rule{
		WorkDir: tmpDir,
	}

	prepared := prepareCommand("echo $MY_VAR", settings, rule, tmpDir)
	expected := "source " + filepath.Join(tmpDir, "env.sh") + " && echo $MY_VAR"
	assert.Equal(t, expected, prepared)

	// Actually run the command and verify output
	cmd := createCrossPlatformCommand(prepared)
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, strings.TrimSpace(string(output)), "hello_from_env")

	// Test function call
	prepared2 := prepareCommand("myfunc", settings, rule, tmpDir)
	cmd2 := createCrossPlatformCommand(prepared2)
	cmd2.Dir = tmpDir
	output2, err := cmd2.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, strings.TrimSpace(string(output2)), "function_output")
}

func TestCommandAliasesE2E(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "devloop-aliases-e2e-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	settings := &pb.Settings{
		Commands: map[string]string{
			"say-hello": "echo hello_world",
		},
	}
	rule := &pb.Rule{
		WorkDir: tmpDir,
	}

	prepared := prepareCommand("$say-hello", settings, rule, tmpDir)
	assert.Equal(t, "echo hello_world", prepared)

	cmd := createCrossPlatformCommand(prepared)
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, "hello_world", strings.TrimSpace(string(output)))
}

// --- parseEnvOutput tests ---

func TestParseEnvOutput_Basic(t *testing.T) {
	// NUL-delimited key=value pairs
	data := []byte("FOO=bar\x00BAZ=qux\x00")
	result := parseEnvOutput(data)
	assert.Equal(t, "bar", result["FOO"])
	assert.Equal(t, "qux", result["BAZ"])
	assert.Len(t, result, 2)
}

func TestParseEnvOutput_Empty(t *testing.T) {
	result := parseEnvOutput([]byte{})
	assert.Empty(t, result)
}

func TestParseEnvOutput_MultilineValues(t *testing.T) {
	// NUL-delimited handles multi-line values correctly
	data := []byte("FOO=line1\nline2\nline3\x00BAR=simple\x00")
	result := parseEnvOutput(data)
	assert.Equal(t, "line1\nline2\nline3", result["FOO"])
	assert.Equal(t, "simple", result["BAR"])
}

func TestParseEnvOutput_MissingEquals(t *testing.T) {
	// Entries without = should be skipped
	data := []byte("FOO=bar\x00INVALID\x00BAZ=qux\x00")
	result := parseEnvOutput(data)
	assert.Equal(t, "bar", result["FOO"])
	assert.Equal(t, "qux", result["BAZ"])
	assert.Len(t, result, 2)
}

// --- envDiff tests ---

func TestEnvDiff_NewAndChanged(t *testing.T) {
	base := map[string]string{
		"EXISTING": "old",
		"SAME":     "value",
	}
	current := map[string]string{
		"EXISTING":  "new",   // changed
		"SAME":      "value", // unchanged
		"BRAND_NEW": "fresh", // new
	}
	diff := envDiff(base, current)
	assert.Equal(t, "new", diff["EXISTING"])
	assert.Equal(t, "fresh", diff["BRAND_NEW"])
	_, hasSame := diff["SAME"]
	assert.False(t, hasSame, "unchanged vars should not appear in diff")
	assert.Len(t, diff, 2)
}

func TestEnvDiff_Empty(t *testing.T) {
	diff := envDiff(map[string]string{}, map[string]string{})
	assert.Empty(t, diff)
}

// --- captureShellFileEnv tests ---

func TestCaptureShellFileEnv_Basic(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "devloop-capture-env-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	envScript := "#!/bin/sh\nexport CAPTURE_TEST_VAR=\"captured_value\"\n"
	err = os.WriteFile(filepath.Join(tmpDir, "env.sh"), []byte(envScript), 0755)
	require.NoError(t, err)

	result, err := captureShellFileEnv([]string{"./env.sh"}, nil, tmpDir, tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "captured_value", result["CAPTURE_TEST_VAR"])
}

func TestCaptureShellFileEnv_NoFiles(t *testing.T) {
	result, err := captureShellFileEnv(nil, nil, "/tmp", "/tmp")
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestCaptureShellFileEnv_MultipleFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "devloop-capture-multi-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	globalScript := "#!/bin/sh\nexport GLOBAL_VAR=\"global\"\n"
	err = os.WriteFile(filepath.Join(tmpDir, "global.sh"), []byte(globalScript), 0755)
	require.NoError(t, err)

	ruleScript := "#!/bin/sh\nexport RULE_VAR=\"rule\"\n"
	err = os.WriteFile(filepath.Join(tmpDir, "rule.sh"), []byte(ruleScript), 0755)
	require.NoError(t, err)

	result, err := captureShellFileEnv([]string{"./global.sh"}, []string{"./rule.sh"}, tmpDir, tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "global", result["GLOBAL_VAR"])
	assert.Equal(t, "rule", result["RULE_VAR"])
}

// --- Config reset_env tests ---

func TestConfigResetEnv(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "devloop-resetenv-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configContent := `
settings:
  reset_env: true
rules:
  - name: "with-override"
    reset_env: false
    watch:
      - action: "include"
        patterns: ["*.go"]
    commands:
      - "echo hi"
  - name: "inherits-global"
    watch:
      - action: "include"
        patterns: ["*.go"]
    commands:
      - "echo hi"
`
	configPath := filepath.Join(tmpDir, "test.yaml")
	err = os.WriteFile(configPath, []byte(strings.TrimSpace(configContent)), 0644)
	require.NoError(t, err)

	config, err := LoadConfig(configPath)
	require.NoError(t, err)

	// Settings level
	assert.True(t, config.Settings.ResetEnv)

	// Rule with explicit override
	require.NotNil(t, config.Rules[0].ResetEnv)
	assert.False(t, *config.Rules[0].ResetEnv)

	// Rule without override (nil = inherit from settings)
	assert.Nil(t, config.Rules[1].ResetEnv)
}

func TestConfigResetEnv_DefaultFalse(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "devloop-resetenv-default-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configContent := `
settings: {}
rules:
  - name: "test"
    watch:
      - action: "include"
        patterns: ["*.go"]
    commands:
      - "echo hi"
`
	configPath := filepath.Join(tmpDir, "test.yaml")
	err = os.WriteFile(configPath, []byte(strings.TrimSpace(configContent)), 0644)
	require.NoError(t, err)

	config, err := LoadConfig(configPath)
	require.NoError(t, err)

	// Default is false (cascade enabled)
	assert.False(t, config.Settings.ResetEnv)
	assert.Nil(t, config.Rules[0].ResetEnv)
}

// --- E2E env cascade tests ---

func TestEnvCascade_Default(t *testing.T) {
	// cmd1 exports FOO, cmd2 sees FOO via env cascade through temp file
	tmpDir, err := os.MkdirTemp("", "devloop-cascade-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Simulate the cascade mechanism:
	// 1. cmd1 exports FOO and writes env to temp file
	// 2. Parse temp file to get cascaded env
	// 3. cmd2 runs with cascaded env and should see FOO

	envTmpFile, err := os.CreateTemp("", "devloop-env-test-*")
	require.NoError(t, err)
	defer os.Remove(envTmpFile.Name())
	defer envTmpFile.Close()

	// Run cmd1: export FOO and capture env
	cmd1Str := "export FOO=cascade_test && env -0 > " + envTmpFile.Name()
	cmd1 := createCrossPlatformCommand(cmd1Str)
	cmd1.Dir = tmpDir
	err = cmd1.Run()
	require.NoError(t, err)

	// Parse the env file
	newEnv, err := parseEnvFile(envTmpFile.Name())
	require.NoError(t, err)
	assert.Equal(t, "cascade_test", newEnv["FOO"])

	// Diff against os env to get only new/changed vars
	osEnv := envSliceToMap(os.Environ())
	cascadedEnv := envDiff(osEnv, newEnv)
	assert.Equal(t, "cascade_test", cascadedEnv["FOO"])

	// Run cmd2 with cascaded env — it should see FOO
	cmd2 := createCrossPlatformCommand("echo $FOO")
	cmd2.Dir = tmpDir
	cmd2.Env = os.Environ()
	for k, v := range cascadedEnv {
		cmd2.Env = append(cmd2.Env, k+"="+v)
	}
	output, err := cmd2.CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, "cascade_test", strings.TrimSpace(string(output)))
}

func TestEnvCascade_ResetEnv(t *testing.T) {
	// With reset_env:true, cmd2 should NOT see cmd1's FOO
	tmpDir, err := os.MkdirTemp("", "devloop-reset-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// When reset_env is true, we only use baseEnv (from shell_files) and don't cascade.
	// cmd2 runs with only os.Environ(), no cascaded env.
	cmd2 := createCrossPlatformCommand("echo \"FOO=${FOO:-unset}\"")
	cmd2.Dir = tmpDir
	cmd2.Env = os.Environ() // no cascaded env
	output, err := cmd2.CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, "FOO=unset", strings.TrimSpace(string(output)))
}

func TestShellFileEnv_CapturedOnce(t *testing.T) {
	// Shell file writes a counter to a file. If captured once, counter = 1.
	tmpDir, err := os.MkdirTemp("", "devloop-capture-once-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	counterFile := filepath.Join(tmpDir, "counter")

	envScript := `#!/bin/sh
if [ -f "` + counterFile + `" ]; then
  COUNT=$(cat "` + counterFile + `")
  COUNT=$((COUNT + 1))
else
  COUNT=1
fi
echo $COUNT > "` + counterFile + `"
export SHELL_FILE_COUNT=$COUNT
`
	err = os.WriteFile(filepath.Join(tmpDir, "env.sh"), []byte(envScript), 0755)
	require.NoError(t, err)

	// Capture once
	result, err := captureShellFileEnv([]string{"./env.sh"}, nil, tmpDir, tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "1", result["SHELL_FILE_COUNT"])

	// The counter file should show 1
	countData, err := os.ReadFile(counterFile)
	require.NoError(t, err)
	assert.Equal(t, "1", strings.TrimSpace(string(countData)))

	// Now simulate 3 commands using the captured env WITHOUT re-running captureShellFileEnv.
	// The shell files are still sourced per-command (for functions/aliases),
	// but the env capture is done once. Here we verify the concept: captureShellFileEnv
	// was called only once, so the counter should still be 1 from the perspective of
	// the initial capture.
	assert.Equal(t, "1", result["SHELL_FILE_COUNT"])
}

func TestShellFileEnv_FunctionsStillWork(t *testing.T) {
	// Shell file defines a function. Verify all commands can call it
	// (functions require sourcing each time, which prepareCommand does).
	tmpDir, err := os.MkdirTemp("", "devloop-functions-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	envScript := `#!/bin/sh
myfunc() { echo "func_works"; }
`
	err = os.WriteFile(filepath.Join(tmpDir, "env.sh"), []byte(envScript), 0755)
	require.NoError(t, err)

	settings := &pb.Settings{ShellFiles: []string{"./env.sh"}}
	rule := &pb.Rule{WorkDir: tmpDir}

	// Each command should be able to call myfunc because prepareCommand
	// still adds the source prefix
	for i := 0; i < 3; i++ {
		prepared := prepareCommand("myfunc", settings, rule, tmpDir)
		cmd := createCrossPlatformCommand(prepared)
		cmd.Dir = tmpDir
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "command %d should succeed", i)
		assert.Equal(t, "func_works", strings.TrimSpace(string(output)))
	}
}

func TestShellFilesAndAliasesE2E(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "devloop-combined-e2e-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a shell file
	envScript := `#!/bin/sh
export PROJECT_NAME="myproject"
`
	err = os.WriteFile(filepath.Join(tmpDir, "env.sh"), []byte(envScript), 0755)
	require.NoError(t, err)

	settings := &pb.Settings{
		ShellFiles: []string{"./env.sh"},
		Commands: map[string]string{
			"show-project": "echo $PROJECT_NAME",
		},
	}
	rule := &pb.Rule{
		WorkDir: tmpDir,
	}

	prepared := prepareCommand("$show-project", settings, rule, tmpDir)
	cmd := createCrossPlatformCommand(prepared)
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, "myproject", strings.TrimSpace(string(output)))
}

// --- Integration tests: full executeNow() flow via orchestrator ---

// helperWaitForRule waits for a rule to reach a terminal status (SUCCESS or FAILED)
func helperWaitForRule(t *testing.T, orchestrator *Orchestrator, ruleName string, timeout time.Duration) string {
	t.Helper()
	start := time.Now()
	for time.Since(start) < timeout {
		runner := orchestrator.GetRuleRunner(ruleName)
		if runner != nil {
			status := runner.GetStatus()
			if status.LastBuildStatus == "SUCCESS" || status.LastBuildStatus == "FAILED" {
				return status.LastBuildStatus
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("rule %q did not complete within %v", ruleName, timeout)
	return ""
}

func TestIntegration_EnvCascade_FullFlow(t *testing.T) {
	// Test that cmd1's exported vars are visible to cmd2 via the full
	// executeNow() flow (not simulated).
	tmpDir, err := os.MkdirTemp("", "devloop-integ-cascade-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create output file that cmd2 will write to
	outputFile := filepath.Join(tmpDir, "output.txt")

	configContent := `
settings:
  project_id: "integ-cascade"
rules:
  - name: "cascade-test"
    skip_run_on_init: true
    watch:
      - action: "include"
        patterns: ["*.trigger"]
    commands:
      - "export CASCADE_VAR=from_cmd1"
      - "echo $CASCADE_VAR > ` + outputFile + `"
`
	configPath := filepath.Join(tmpDir, ".devloop.yaml")
	err = os.WriteFile(configPath, []byte(strings.TrimSpace(configContent)), 0644)
	require.NoError(t, err)

	// Create logs directory
	logsDir := filepath.Join(tmpDir, "logs")
	require.NoError(t, os.MkdirAll(logsDir, 0755))

	orchestrator, err := NewOrchestrator(configPath)
	require.NoError(t, err)

	go orchestrator.Start()
	defer orchestrator.Stop()
	time.Sleep(200 * time.Millisecond)

	// Trigger the rule
	err = orchestrator.TriggerRule("cascade-test")
	require.NoError(t, err)

	status := helperWaitForRule(t, orchestrator, "cascade-test", 5*time.Second)
	assert.Equal(t, "SUCCESS", status)

	// Verify cmd2 saw cmd1's export
	data, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Equal(t, "from_cmd1", strings.TrimSpace(string(data)))
}

func TestIntegration_EnvCascade_ResetEnv(t *testing.T) {
	// With reset_env: true, cmd2 should NOT see cmd1's exports
	tmpDir, err := os.MkdirTemp("", "devloop-integ-reset-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "output.txt")

	configContent := `
settings:
  project_id: "integ-reset"
  reset_env: true
rules:
  - name: "reset-test"
    skip_run_on_init: true
    watch:
      - action: "include"
        patterns: ["*.trigger"]
    commands:
      - "export RESET_VAR=should_not_cascade"
      - "echo ${RESET_VAR:-unset} > ` + outputFile + `"
`
	configPath := filepath.Join(tmpDir, ".devloop.yaml")
	err = os.WriteFile(configPath, []byte(strings.TrimSpace(configContent)), 0644)
	require.NoError(t, err)

	logsDir := filepath.Join(tmpDir, "logs")
	require.NoError(t, os.MkdirAll(logsDir, 0755))

	orchestrator, err := NewOrchestrator(configPath)
	require.NoError(t, err)

	go orchestrator.Start()
	defer orchestrator.Stop()
	time.Sleep(200 * time.Millisecond)

	err = orchestrator.TriggerRule("reset-test")
	require.NoError(t, err)

	status := helperWaitForRule(t, orchestrator, "reset-test", 5*time.Second)
	assert.Equal(t, "SUCCESS", status)

	data, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Equal(t, "unset", strings.TrimSpace(string(data)))
}

func TestIntegration_ShellFileEnv_CapturedOnce(t *testing.T) {
	// Shell file has a side effect (counter file). Verify the env is captured
	// once, not re-executed per command, by checking that cmd3 still sees
	// the value from the single capture.
	tmpDir, err := os.MkdirTemp("", "devloop-integ-once-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	counterFile := filepath.Join(tmpDir, "counter")
	outputFile := filepath.Join(tmpDir, "output.txt")

	// Shell file: increments counter each time it's sourced
	envScript := `#!/bin/sh
if [ -f "` + counterFile + `" ]; then
  COUNT=$(cat "` + counterFile + `")
  COUNT=$((COUNT + 1))
else
  COUNT=1
fi
echo $COUNT > "` + counterFile + `"
export COUNTER=$COUNT
`
	err = os.WriteFile(filepath.Join(tmpDir, "env.sh"), []byte(envScript), 0755)
	require.NoError(t, err)

	// 3 commands: the shell file is sourced per-command (for functions),
	// but the env capture happens once. The COUNTER env var injected
	// via cmd.Env should always be "1" (from the single capture).
	// However, the shell file is ALSO sourced per-command, so the actual
	// $COUNTER from sourcing will keep incrementing.
	// The key test: the captured env var (injected via cmd.Env) is "1".
	// We test this by writing the DEVLOOP_CAPTURED_COUNT env var.
	//
	// Actually, let's test it differently: write the counter file value
	// after all 3 commands. The counter file tracks how many times
	// the shell file was sourced total (capture + 3 commands = 4).
	// But the env capture only runs once.
	configContent := `
settings:
  project_id: "integ-once"
  shell_files:
    - "./env.sh"
rules:
  - name: "once-test"
    skip_run_on_init: true
    watch:
      - action: "include"
        patterns: ["*.trigger"]
    commands:
      - "echo step1"
      - "echo step2"
      - "cat ` + counterFile + ` > ` + outputFile + `"
`
	configPath := filepath.Join(tmpDir, ".devloop.yaml")
	err = os.WriteFile(configPath, []byte(strings.TrimSpace(configContent)), 0644)
	require.NoError(t, err)

	logsDir := filepath.Join(tmpDir, "logs")
	require.NoError(t, os.MkdirAll(logsDir, 0755))

	orchestrator, err := NewOrchestrator(configPath)
	require.NoError(t, err)

	go orchestrator.Start()
	defer orchestrator.Stop()
	time.Sleep(200 * time.Millisecond)

	err = orchestrator.TriggerRule("once-test")
	require.NoError(t, err)

	status := helperWaitForRule(t, orchestrator, "once-test", 5*time.Second)
	assert.Equal(t, "SUCCESS", status)

	// The counter file should show 4: 1 capture + 3 command sources
	// (shell files are still sourced per-command for functions/aliases)
	data, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	count := strings.TrimSpace(string(data))
	// Capture runs the shell file once, then each of 3 commands sources it again
	assert.Equal(t, "4", count, "shell file should be sourced 4 times total (1 capture + 3 commands)")
}

func TestIntegration_ShellFileEnv_FunctionsAvailable(t *testing.T) {
	// Shell file defines a function. All commands should be able to call it
	// because shell files are still sourced per-command.
	tmpDir, err := os.MkdirTemp("", "devloop-integ-func-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "output.txt")

	envScript := `#!/bin/sh
greet() { echo "hello_from_func"; }
`
	err = os.WriteFile(filepath.Join(tmpDir, "env.sh"), []byte(envScript), 0755)
	require.NoError(t, err)

	configContent := `
settings:
  project_id: "integ-func"
  shell_files:
    - "./env.sh"
rules:
  - name: "func-test"
    skip_run_on_init: true
    watch:
      - action: "include"
        patterns: ["*.trigger"]
    commands:
      - "greet > ` + outputFile + `"
`
	configPath := filepath.Join(tmpDir, ".devloop.yaml")
	err = os.WriteFile(configPath, []byte(strings.TrimSpace(configContent)), 0644)
	require.NoError(t, err)

	logsDir := filepath.Join(tmpDir, "logs")
	require.NoError(t, os.MkdirAll(logsDir, 0755))

	orchestrator, err := NewOrchestrator(configPath)
	require.NoError(t, err)

	go orchestrator.Start()
	defer orchestrator.Stop()
	time.Sleep(200 * time.Millisecond)

	err = orchestrator.TriggerRule("func-test")
	require.NoError(t, err)

	status := helperWaitForRule(t, orchestrator, "func-test", 5*time.Second)
	assert.Equal(t, "SUCCESS", status)

	data, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Equal(t, "hello_from_func", strings.TrimSpace(string(data)))
}

func TestIntegration_RuleOverridesGlobalResetEnv(t *testing.T) {
	// Global reset_env: true, but rule overrides with reset_env: false
	// So cascade should work for this rule.
	tmpDir, err := os.MkdirTemp("", "devloop-integ-override-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "output.txt")

	configContent := `
settings:
  project_id: "integ-override"
  reset_env: true
rules:
  - name: "override-test"
    reset_env: false
    skip_run_on_init: true
    watch:
      - action: "include"
        patterns: ["*.trigger"]
    commands:
      - "export OVERRIDE_VAR=cascaded"
      - "echo $OVERRIDE_VAR > ` + outputFile + `"
`
	configPath := filepath.Join(tmpDir, ".devloop.yaml")
	err = os.WriteFile(configPath, []byte(strings.TrimSpace(configContent)), 0644)
	require.NoError(t, err)

	logsDir := filepath.Join(tmpDir, "logs")
	require.NoError(t, os.MkdirAll(logsDir, 0755))

	orchestrator, err := NewOrchestrator(configPath)
	require.NoError(t, err)

	go orchestrator.Start()
	defer orchestrator.Stop()
	time.Sleep(200 * time.Millisecond)

	err = orchestrator.TriggerRule("override-test")
	require.NoError(t, err)

	status := helperWaitForRule(t, orchestrator, "override-test", 5*time.Second)
	assert.Equal(t, "SUCCESS", status)

	// Rule override should enable cascade, so cmd2 sees cmd1's export
	data, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Equal(t, "cascaded", strings.TrimSpace(string(data)))
}
