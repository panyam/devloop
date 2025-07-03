package agent

import (
	"testing"
	"time"

	"github.com/panyam/devloop/testhelpers"
)

// withTestContext is a wrapper around testhelpers.WithTestContext for backward compatibility
func withTestContext(t *testing.T, timeout time.Duration, testFunc func(t *testing.T, tmpDir string)) {
	testhelpers.WithTestContext(t, timeout, testFunc)
}
