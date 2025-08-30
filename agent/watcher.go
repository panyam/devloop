package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
	eventChan   chan string
	stopChan    chan struct{}
	stoppedChan chan struct{}
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
		eventChan:   make(chan string),
	}
}

func (w *Watcher) EventChan() chan string {
	return w.eventChan
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

	// Convert absolute directory path to relative path for pattern matching
	baseDir := w.getBaseDirectory()
	relativeDirPath := dirPath

	// If dirPath is absolute and starts with baseDir, make it relative
	if filepath.IsAbs(dirPath) && strings.HasPrefix(dirPath, baseDir) {
		relativeDirPath = strings.TrimPrefix(dirPath, baseDir)
		relativeDirPath = strings.TrimPrefix(relativeDirPath, string(filepath.Separator))

		// If relative path is empty, it means we're checking the base directory itself
		// Use "." to represent the current directory for pattern matching
		if relativeDirPath == "" {
			relativeDirPath = "."
		}
	}

	hasIncludeMatch := false

	// Check all patterns to see if directory should be watched
	for _, matcher := range w.rule.Watch {
		for _, pattern := range matcher.Patterns {
			// Patterns are already relative, no need to resolve
			if w.shouldWatchDirectoryForPattern(pattern, relativeDirPath) {
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

// shouldWatchDirectoryForPattern determines if we should watch a directory for a given pattern
// This is specifically for directory watching decisions, not file matching
func (w *Watcher) shouldWatchDirectoryForPattern(pattern, dirPath string) bool {
	// Clean paths for consistent comparison
	pattern = filepath.Clean(pattern)
	dirPath = filepath.Clean(dirPath)

	// If checking the current directory ("."), check if pattern could match files directly
	if dirPath == "." {
		return true // Always watch current directory if we have any patterns
	}

	// If pattern is exact match to directory, it should be watched
	if pattern == dirPath {
		return true
	}

	// If pattern contains the directory as a prefix, we MUST watch this directory
	// For example: "web/server/*.go" requires watching "web/" to detect "web/server/" changes
	prefix := dirPath + string(filepath.Separator)
	if strings.HasPrefix(pattern, prefix) {
		return true
	}

	// For glob patterns, check if this directory could contain matching files
	// For example: "**/*.go" should cause watching any directory because it could contain .go files
	// But "src/**/*.go" should only watch src/ and its subdirectories, not lib/ or docs/
	if strings.Contains(pattern, "**") {
		// Extract the prefix before **
		starIndex := strings.Index(pattern, "**")
		if starIndex == 0 {
			// Pattern starts with **, like "**/*.go" - can match any directory
			return true
		} else {
			// Pattern has prefix before **, like "src/**/*.go"
			prefix := pattern[:starIndex]
			prefix = strings.TrimSuffix(prefix, "/")

			// Directory should be watched if it's the prefix or under the prefix
			if dirPath == prefix {
				return true
			}
			if strings.HasPrefix(dirPath, prefix+"/") {
				return true
			}
			return false
		}
	}

	return false
}

// patternCouldMatchInDirectory checks if a pattern could potentially match files in a directory
func (w *Watcher) patternCouldMatchInDirectory(pattern, dirPath string) bool {
	// Clean paths for consistent comparison
	pattern = filepath.Clean(pattern)
	dirPath = filepath.Clean(dirPath)

	// If checking the current directory ("."), check if pattern could match files directly
	if dirPath == "." {
		// Pattern like "watch-me.txt" should match files in current directory
		if !strings.Contains(pattern, string(filepath.Separator)) {
			return true
		}
		// Pattern like "src/**/*.go" could also match starting from current directory
		return true
	}

	// If pattern is exact match to directory, it should be watched
	if pattern == dirPath {
		return true
	}

	// If pattern contains the directory as a prefix, it could match files inside
	prefix := dirPath + string(filepath.Separator)
	if strings.HasPrefix(pattern, prefix) {
		// Additional check: the pattern should not specify a deeper directory structure
		// UNLESS it uses ** wildcards which can match nested directories
		remaining := strings.TrimPrefix(pattern, prefix)

		// If remaining part contains "**", it can match at any depth
		if strings.Contains(remaining, "**") {
			return true
		}

		// If remaining part contains a directory separator without **, it's targeting a subdirectory
		// For example, "web/server/*.go" should NOT match files in "web/" directory
		if !strings.Contains(remaining, string(filepath.Separator)) {
			return true
		}
	}

	// For patterns with **, analyze the non-glob prefix to constrain matching
	if strings.Contains(pattern, "**") {
		if strings.HasPrefix(pattern, "**/") {
			// Pattern like "**/*.go" can match in any directory (no constraint)
			return true
		}

		// Pattern like "lib/**/*.go" - extract the constraining prefix "lib"
		parts := strings.Split(pattern, "/**")
		if len(parts) > 0 {
			constrainingPrefix := parts[0]
			// Check if this directory is within the constraining prefix
			if dirPath == constrainingPrefix || strings.HasPrefix(dirPath, constrainingPrefix+"/") {
				return true
			}
		}

		return false // ** pattern doesn't apply to this directory
	}

	// For simple glob patterns (*, ?, [])
	if strings.ContainsAny(pattern, "*?[]") {
		// If pattern has no directory separators, it could match files in any directory
		if !strings.Contains(pattern, string(filepath.Separator)) {
			return true
		}
	}

	return false
}

// watchFiles processes file system events
func (w *Watcher) watchFiles() {
	defer func() {
		close(w.eventChan)
		w.eventChan = nil
		close(w.stoppedChan)
	}()

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

	if w.eventChan != nil {
		w.eventChan <- event.Name
	}
}

// fileMatchesRule checks if a file matches this rule's patterns
func (w *Watcher) fileMatchesRule(filePath string) bool {
	// Convert absolute file path to relative path for pattern matching
	baseDir := w.getBaseDirectory()
	relativeFilePath := filePath

	// If filePath is absolute and starts with baseDir, make it relative
	if filepath.IsAbs(filePath) && strings.HasPrefix(filePath, baseDir) {
		relativeFilePath = strings.TrimPrefix(filePath, baseDir)
		relativeFilePath = strings.TrimPrefix(relativeFilePath, string(filepath.Separator))
	}

	// Use the existing rule matching logic with relative path
	matcher := RuleMatches(w.rule, relativeFilePath, w.configPath)
	return matcher != nil && matcher.Action == "include"
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
