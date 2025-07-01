package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gobwas/glob"
	"gopkg.in/yaml.v3"
)

// Config represents the top-level structure of the .devloop.yaml file.
type Config struct {
	Settings Settings `yaml:"settings"`
	Rules    []Rule   `yaml:"rules"`
}

// Settings defines global settings for devloop.
type Settings struct {
	PrefixLogs      bool `yaml:"prefix_logs"`
	PrefixMaxLength int  `yaml:"prefix_max_length"`
}

// Rule defines a single watch-and-run rule.
type Rule struct {
	Name     string            `yaml:"name"`
	Prefix   string            `yaml:"prefix,omitempty"`
	Commands []string          `yaml:"commands"`
	Watch    []*Matcher        `yaml:"watch"`
	Env      map[string]string `yaml:"env,omitempty"`
	WorkDir  string            `yaml:"workdir,omitempty"`
}

// Matcher defines a single include or exclude directive using glob patterns.
type Matcher struct {
	Action   string   `yaml:"action"` // Should be "include" or "exclude"
	Patterns []string `yaml:"patterns"`
	Globs    []glob.Glob
}

func (m *Matcher) Matches(filePath string) bool {
	if m.Globs == nil {
		for _, pattern := range m.Patterns {
			g, err := glob.Compile(pattern)
			if err != nil {
				panic(fmt.Sprintf("Error compiling glob pattern %q: %v", pattern, err))
			} else {
				m.Globs = append(m.Globs, g)
			}
		}
	}
	for _, g := range m.Globs {
		if g.Match(filePath) {
			return true
		}
	}
	return false
}

// Matches checks if the given file path matches the rule's watch criteria.
func (r *Rule) Match(filePath string) *Matcher {
	for _, matcher := range r.Watch {
		if matcher.Matches(filePath) {
			return matcher
		}
	}
	return nil
}

// executeCommands runs the commands associated with a rule.
func (o *Orchestrator) executeCommands(rule Rule) {
	log.Printf("Executing commands for rule %q", rule.Name)

	// Terminate any previously running commands for this rule
	if cmds, ok := o.runningCommands[rule.Name]; ok {
		for _, cmd := range cmds {
			if cmd != nil && cmd.Process != nil {
				log.Printf("Terminating previous process group %d for rule %q", cmd.Process.Pid, rule.Name)
				// Send SIGTERM to the process group
				err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
				if err != nil {
					log.Printf("Error sending SIGTERM to process group %d for rule %q: %v", cmd.Process.Pid, rule.Name, err)
				}
				// Wait for the process to exit
				go func(c *exec.Cmd) {
					_ = c.Wait() // Wait for the process to actually exit
					log.Printf("Previous process group %d for rule %q terminated.", c.Process.Pid, rule.Name)
				}(cmd)
			}
		}
	}
	o.runningCommands[rule.Name] = []*exec.Cmd{} // Clear previous commands

	var currentCmds []*exec.Cmd
	for _, cmdStr := range rule.Commands {
		log.Printf("  Running command: %s", cmdStr)
		cmd := exec.Command("bash", "-c", cmdStr)

		if o.Config.Settings.PrefixLogs {
			prefix := rule.Name
			if rule.Prefix != "" {
				prefix = rule.Prefix
			}

			if o.Config.Settings.PrefixMaxLength > 0 {
				if len(prefix) > o.Config.Settings.PrefixMaxLength {
					prefix = prefix[:o.Config.Settings.PrefixMaxLength]
				} else {
					for len(prefix) < o.Config.Settings.PrefixMaxLength {
						prefix += " "
					}
				}
			}

			cmd.Stdout = &PrefixWriter{writer: os.Stdout, prefix: "[" + prefix + "] "}
			cmd.Stderr = &PrefixWriter{writer: os.Stderr, prefix: "[" + prefix + "] "}
		} else {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}

		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // Set process group ID

		// Set working directory if specified
		if rule.WorkDir != "" {
			cmd.Dir = rule.WorkDir
		}

		// Set environment variables
		cmd.Env = os.Environ() // Inherit parent environment
		for key, value := range rule.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}
		err := cmd.Start()
		if err != nil {
			log.Printf("  Command %q failed to start for rule %q: %v", cmdStr, rule.Name, err)
			continue
		}
		currentCmds = append(currentCmds, cmd)

		// For long-running commands, we don't wait here. They run in background.
		// For now, we'll assume all commands might be long-running and don't wait.
	}
	o.runningCommands[rule.Name] = currentCmds
}

// Orchestrator manages file watching and rule execution.
type Orchestrator struct {
	Config           *Config
	Watcher          *fsnotify.Watcher
	done             chan bool
	debounceTimers   map[string]*time.Timer
	debouncedEvents  chan Rule
	debounceDuration time.Duration
	runningCommands  map[string][]*exec.Cmd // Track running commands by rule name
}

// debounce manages the debounce timer for a given rule.
func (o *Orchestrator) debounce(rule Rule) {
	if timer, ok := o.debounceTimers[rule.Name]; ok {
		timer.Stop() // Reset the existing timer
	}
	o.debounceTimers[rule.Name] = time.AfterFunc(o.debounceDuration, func() {
		o.debouncedEvents <- rule // Send rule to debouncedEvents channel after duration
	})
}

// LoadConfig reads and unmarshals the .devloop.yaml configuration file.
func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %q", filePath)
		}
		return nil, fmt.Errorf("failed to read config file %q: %w", filePath, err)
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", filePath, err)
	}

	return &config, nil
}

// NewOrchestrator creates a new Orchestrator instance.
func NewOrchestrator(configPath string) (*Orchestrator, error) {
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	return &Orchestrator{
		Config:           config,
		Watcher:          watcher,
		done:             make(chan bool),
		debounceTimers:   make(map[string]*time.Timer),
		debouncedEvents:  make(chan Rule),
		debounceDuration: 500 * time.Millisecond, // Default debounce duration
		runningCommands:  make(map[string][]*exec.Cmd),
	}, nil
}

// Start begins the file watching and event processing.
func (o *Orchestrator) Start() error {
	// Goroutine for processing file system events
	go func() {
		for {
			select {
			case event, ok := <-o.Watcher.Events:
				if !ok {
					return
				}
				log.Println("event:", event)
				if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
					log.Printf("File event detected: %s", event.Name)
					for _, rule := range o.Config.Rules {
						if m := rule.Match(event.Name); m != nil {
							log.Printf("Rule %q matched for file %q. Debouncing...", rule.Name, event.Name)
							o.debounce(rule)
						}
					}
				} else if event.Op&fsnotify.Remove == fsnotify.Remove {
					log.Println("removed file/directory:", event.Name)
				} else if event.Op&fsnotify.Rename == fsnotify.Rename {
					log.Println("renamed file/directory:", event.Name)
				} else if event.Op&fsnotify.Chmod == fsnotify.Chmod {
					log.Println("chmodded file/directory:", event.Name)
				}
			case err, ok := <-o.Watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			}
		}
	}()

	// Goroutine for processing debounced events
	go func() {
		for {
			select {
			case rule := <-o.debouncedEvents:
				o.executeCommands(rule)
			case <-o.done:
				return
			}
		}
	}()

	// Add the current directory to the watcher initially.
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Log the error but continue walking if it's a permission error
			if os.IsPermission(err) {
				log.Printf("Permission denied for path %q: %v", path, err)
				return nil // Continue walking
			}
			return fmt.Errorf("error walking path %q: %w", path, err)
		}
		if info.IsDir() {
			err = o.Watcher.Add(path)
			if err != nil {
				// Log the error but continue if it's a common watcher error
				log.Printf("Error adding watcher for directory %q: %v", path, err)
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to add initial watches: %w", err)
	}

	log.Println("Watching for file changes...")
	<-o.done // Keep the main goroutine alive until done is closed

	return nil
}

// Stop gracefully shuts down the orchestrator.
func (o *Orchestrator) Stop() error {
	log.Println("Stopping orchestrator...")
	close(o.done)

	// Terminate all running commands
	for ruleName, cmds := range o.runningCommands {
		for _, cmd := range cmds {
			if cmd != nil && cmd.Process != nil {
				log.Printf("Terminating process group %d for rule %q during shutdown", cmd.Process.Pid, ruleName)
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			}
		}
	}

	return o.Watcher.Close()
}
