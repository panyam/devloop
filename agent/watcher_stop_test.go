package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
)

// TestWatcherStopDuringEvents stops a watcher while files are being created
// and removed under it. Remove events take the watcher's mutex, so Stop must
// not hold that mutex while it waits for the event goroutine to exit.
func TestWatcherStopDuringEvents(t *testing.T) {
	for i := range 20 {
		dir := t.TempDir()
		rule := &pb.Rule{
			Name:  "stop-test",
			Watch: []*pb.RuleMatcher{{Action: "include", Patterns: []string{"**/*"}}},
		}
		w := NewWatcher(rule, filepath.Join(dir, ".devloop.yaml"), false)
		if err := w.Start(); err != nil {
			t.Fatal(err)
		}

		events := w.EventChan()
		go func() {
			for range events {
			}
		}()

		quit := make(chan struct{})
		var churn sync.WaitGroup
		for g := range 2 {
			churn.Add(1)
			go func() {
				defer churn.Done()
				for n := 0; ; n++ {
					select {
					case <-quit:
						return
					default:
					}
					p := filepath.Join(dir, fmt.Sprintf("f%d-%d", g, n%10))
					os.WriteFile(p, nil, 0644)
					os.Remove(p)
				}
			}()
		}
		time.Sleep(20 * time.Millisecond)

		stopped := make(chan error, 1)
		go func() { stopped <- w.Stop() }()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			close(quit)
			t.Fatalf("iteration %d: Stop did not return while events were arriving", i)
		}
		close(quit)
		churn.Wait()
	}
}
