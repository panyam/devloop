package gateway

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestConvertAirToml(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "devloop_converter_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a dummy .air.toml file
	airTomlPath := filepath.Join(tmpDir, ".air.toml")
	airTomlContent := `
root = "."
tmp_dir = "tmp"

[build]
cmd = "go build -o ./tmp/main ."
bin = "tmp/main"
pre_cmd = ["echo 'building...'"]
post_cmd = ["echo 'build finished'"]
include_ext = ["go", "tpl", "tmpl", "html"]
exclude_dir = ["assets", "tmp", "vendor"]
include_dir = ["cmd"]
exclude_file = ["main_test.go"]
exclude_regex = ["_test.go"]
`
	err = os.WriteFile(airTomlPath, []byte(airTomlContent), 0644)
	assert.NoError(t, err)

	// Capture the output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run the conversion
	err = ConvertAirToml(airTomlPath)
	assert.NoError(t, err)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify the output
	assert.Contains(t, output, "Warning: The 'exclude_regex' directive is not supported and will be ignored.")

	var generatedRule Rule
	err = yaml.Unmarshal([]byte(output), &generatedRule)
	if err != nil {
		// If there's an error, it might be because of the warning message.
		// Let's try to parse the output after the warning message.
		parts := strings.Split(output, "\n")
		yamlPart := strings.Join(parts[1:], "\n")
		err = yaml.Unmarshal([]byte(yamlPart), &generatedRule)
		assert.NoError(t, err)
	}

	assert.Equal(t, "Imported from .air.toml", generatedRule.Name)
	assert.Equal(t, []string{"echo 'building...'", "go build -o ./tmp/main .", "echo 'build finished'"}, generatedRule.Commands)
	assert.Len(t, generatedRule.Watch, 2)
	assert.Equal(t, "include", generatedRule.Watch[0].Action)
	assert.Equal(t, []string{"**/*.go", "**/*.tpl", "**/*.tmpl", "**/*.html", "cmd/**/*"}, generatedRule.Watch[0].Patterns)
	assert.Equal(t, "exclude", generatedRule.Watch[1].Action)
	assert.Equal(t, []string{"assets/**/*", "tmp/**/*", "vendor/**/*", "main_test.go"}, generatedRule.Watch[1].Patterns)
}
