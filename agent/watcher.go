package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fsnotify/fsnotify"
	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"github.com/panyam/devloop/utils"
)

// Watcher manages file system watching for a single rule
type Watcher struct {
	rule       *pb.Rule
	configPath string
	verbose    bool

	// File watching
	fsWatcher   *fsnotify.Watcher
	watchedDirs map[string]bool
	mutex       sync.RWMutex

	// Event handling
	eventHandler func(string) // Called when a relevant file changes
	stopChan     chan struct{}
	stoppedChan  chan struct{}
}

// NewWatcher creates a new Watcher for the given rule
func NewWatcher(rule *pb.Rule, configPath string, verbose bool) *Watcher {
	return &Watcher{
		rule:        rule,
		configPath:  configPath,
		verbose:     verbose,
		watchedDirs: make(map[string]bool),
		stopChan:    make(chan struct{}),
		stoppedChan: make(chan struct{}),
	}
}

// SetEventHandler sets the callback function for file change events
func (w *Watcher) SetEventHandler(handler func(string)) {
	w.eventHandler = handler
}

// Start initializes the file watcher and begins monitoring
func (w *Watcher) Start() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	// Create the fsnotify watcher
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher for rule %q: %w", w.rule.Name, err)
	}
	w.fsWatcher = fsWatcher

	// Setup file watching based on rule patterns
	if err := w.setupFileWatching(); err != nil {
		w.fsWatcher.Close()
		return fmt.Errorf("failed to setup file watching for rule %q: %w", w.rule.Name, err)
	}

	// Start the event processing goroutine
	go w.watchFiles()

	if w.verbose {
		utils.LogDevloop("[%s] File watcher started", w.rule.Name)
	}

	return nil
}

// Stop stops the file watcher
func (w *Watcher) Stop() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.fsWatcher == nil {
		return nil // Already stopped
	}

	// Signal stop and wait for goroutine to finish
	close(w.stopChan)
	<-w.stoppedChan

	// Close the fsnotify watcher
	err := w.fsWatcher.Close()
	w.fsWatcher = nil
	w.watchedDirs = make(map[string]bool)

	if w.verbose {
		utils.LogDevloop("[%s] File watcher stopped", w.rule.Name)
	}

	return err
}

// setupFileWatching determines which directories to watch based on rule patterns
func (w *Watcher) setupFileWatching() error {
	// Get the base directory for this rule
	baseDir := w.getBaseDirectory()

	if w.verbose {
		utils.LogDevloop("[%s] Setting up file watching from base directory: %s", w.rule.Name, baseDir)
	}

	// Walk the directory tree and determine what to watch
	return filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			shouldWatch := w.shouldWatchDirectory(path)
			if shouldWatch {
				if err := w.fsWatcher.Add(path); err != nil {
					utils.LogDevloop("[%s] Error watching %s: %v", w.rule.Name, path, err)
				} else {
					w.watchedDirs[path] = true
					if w.verbose {
						utils.LogDevloop("[%s] Watching directory: %s", w.rule.Name, path)
					}
				}
			} else {
				if w.verbose {
					utils.LogDevloop("[%s] Skipping directory: %s", w.rule.Name, path)
				}
				return filepath.SkipDir
			}
		}
		return nil
	})
}

// getBaseDirectory returns the base directory for pattern resolution
func (w *Watcher) getBaseDirectory() string {
	// If rule has a work_dir, use it as the base
	if w.rule.WorkDir != "" {
		if filepath.IsAbs(w.rule.WorkDir) {
			return w.rule.WorkDir
		} else {
			// work_dir is relative to config file location
			return filepath.Join(filepath.Dir(w.configPath), w.rule.WorkDir)
		}
	}

	// No work_dir specified, use config file directory
	return filepath.Dir(w.configPath)
}

// shouldWatchDirectory determines if a directory should be watched based on this rule's patterns
func (w *Watcher) shouldWatchDirectory(dirPath string) bool {
	// Directory watching logic: We should watch a directory if ANY include pattern
	// could match files in it. Exclude patterns only affect individual files, not directories.
	
	hasIncludeMatch := false
	
	// Check all patterns to see if directory should be watched
	for _, matcher := range w.rule.Watch {
		for _, pattern := range matcher.Patterns {
			resolvedPattern := resolvePattern(pattern, w.rule, w.configPath)

			if w.patternCouldMatchInDirectory(resolvedPattern, dirPath) {
				if matcher.Action == "include" {
					hasIncludeMatch = true
				}
			}
		}
	}
	
	// Watch directory only if there's an include pattern that could match files in it
	if hasIncludeMatch {
		return true
	}

	// If no include patterns could match, don't watch the directory
	// (This prevents watching directories just because exclude patterns could match)
	return false
}

// patternCouldMatchInDirectory checks if a pattern could potentially match files in a directory
func (w *Watcher) patternCouldMatchInDirectory(pattern, dirPath string) bool {
	// Clean paths for consistent comparison
	pattern = filepath.Clean(pattern)
	dirPath = filepath.Clean(dirPath)

	// If pattern is exact match to directory, it should be watched
	if pattern == dirPath {
		return true
	}

	// If pattern contains the directory as a prefix, it could match files inside
	if strings.HasPrefix(pattern, dirPath+string(filepath.Separator)) {
		return true
	}

	// Check if this is a glob pattern (contains wildcards)
	isGlob := strings.ContainsAny(pattern, "*?[]")

	if isGlob {
		// For glob patterns, use proper glob matching to check if it could match files in directory
		testFile := filepath.Join(dirPath, "test.txt")
		if matched, _ := doublestar.Match(pattern, testFile); matched {
			return true
		}

		// Check with common file extensions for glob patterns
		extensions := []string{".go", ".js", ".ts", ".py", ".java", ".cpp", ".c", ".h", ".log", ".md"}
		for _, ext := range extensions {
			testFile := filepath.Join(dirPath, "test"+ext)
			if matched, _ := doublestar.Match(pattern, testFile); matched {
				return true
			}
		}
	}

	return false
}

// watchFiles processes file system events
func (w *Watcher) watchFiles() {
	defer close(w.stoppedChan)

	for {
		select {
		case <-w.stopChan:
			return

		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}

			w.handleFileEvent(event)

		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			utils.LogDevloop("[%s] File watcher error: %v", w.rule.Name, err)
		}
	}
}

// handleFileEvent processes a single file system event
func (w *Watcher) handleFileEvent(event fsnotify.Event) {
	// Check if this file matches our rule patterns
	if !w.fileMatchesRule(event.Name) {
		return // File doesn't match this rule
	}

	if w.verbose {
		utils.LogDevloop("[%s] File change detected: %s", w.rule.Name, event.Name)
	}

	// Handle directory creation/deletion
	if event.Op&fsnotify.Create == fsnotify.Create {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			// New directory created, check if we should watch it
			if w.shouldWatchDirectory(event.Name) {
				w.mutex.Lock()
				if err := w.fsWatcher.Add(event.Name); err != nil {
					utils.LogDevloop("[%s] Error watching new directory %s: %v", w.rule.Name, event.Name, err)
				} else {
					w.watchedDirs[event.Name] = true
					if w.verbose {
						utils.LogDevloop("[%s] Started watching new directory: %s", w.rule.Name, event.Name)
					}
				}
				w.mutex.Unlock()
			}
		}
	}

	if event.Op&fsnotify.Remove == fsnotify.Remove {
		// Directory/file removed, clean up if it was a watched directory
		w.mutex.Lock()
		if w.watchedDirs[event.Name] {
			delete(w.watchedDirs, event.Name)
			if w.verbose {
				utils.LogDevloop("[%s] Stopped watching removed directory: %s", w.rule.Name, event.Name)
			}
		}
		w.mutex.Unlock()
	}

	// Call the event handler if one is set
	if w.eventHandler != nil {
		w.eventHandler(event.Name)
	}
}

// fileMatchesRule checks if a file matches this rule's patterns
func (w *Watcher) fileMatchesRule(filePath string) bool {
	// Use the existing rule matching logic
	return RuleMatches(w.rule, filePath, w.configPath) != nil
}

// GetWatchedDirectories returns a copy of currently watched directories
func (w *Watcher) GetWatchedDirectories() []string {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	dirs := make([]string, 0, len(w.watchedDirs))
	for dir := range w.watchedDirs {
		dirs = append(dirs, dir)
	}
	return dirs
}
