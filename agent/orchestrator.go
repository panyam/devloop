package agent

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fsnotify/fsnotify"
	"github.com/panyam/gocurrent"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"github.com/panyam/devloop/utils"
)

// Orchestrator manages file watching and delegates execution to RuleRunners
type Orchestrator struct {
	projectID    string
	ConfigPath   string
	Config       *pb.Config
	Verbose      bool
	Watcher      *fsnotify.Watcher
	LogManager   *utils.LogManager
	ColorManager *utils.ColorManager

	// Rule management
	ruleRunners  map[string]*RuleRunner
	runnersMutex sync.RWMutex

	// Control channels
	done     chan bool
	doneOnce sync.Once

	// Debounce settings
	debounceDuration time.Duration
}

// NewOrchestrator creates a new orchestrator instance for managing file watching and rule execution.
//
// The orchestrator handles:
// - Loading and validating configuration from configPath
// - Setting up file system watching for specified patterns
// - Managing rule execution and process lifecycle
// - Optional gateway communication if gatewayAddr is provided
//
// Parameters:
//   - configPath: Path to the .devloop.yaml configuration file
//   - gatewayAddr: Optional gateway address for distributed mode (empty for standalone)
//
// Returns an orchestrator instance ready to be started, or an error if
// configuration loading or initialization fails.
//
// Example:
//
//	// Standalone mode
//	orchestrator, err := NewOrchestrator(".devloop.yaml", "")
//
//	// Agent mode (connect to gateway)
//	orchestrator, err := NewOrchestrator(".devloop.yaml", "localhost:50051")
func NewOrchestrator(configPath string) (*Orchestrator, error) {
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("could not determine absolute path for config: %w", err)
	}

	config, err := LoadConfig(absConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	// Determine project ID - use config value if provided, otherwise generate from path
	var projectID string
	if config.Settings.ProjectId != "" {
		projectID = config.Settings.ProjectId
	} else {
		// Generate project ID from path
		projectRoot := filepath.Dir(absConfigPath)
		hasher := sha1.New()
		hasher.Write([]byte(projectRoot))
		projectID = hex.EncodeToString(hasher.Sum(nil))[:16]
	}

	orchestrator := &Orchestrator{
		ConfigPath:  absConfigPath,
		Config:      config,
		Watcher:     watcher,
		projectID:   projectID,
		done:        make(chan bool),
		ruleRunners: make(map[string]*RuleRunner),
		// gatewaySendChan:  make(chan *pb.DevloopMessage, 100),
		// responseChan:     make(map[string]chan *pb.DevloopMessage),
		debounceDuration: 500 * time.Millisecond,
	}

	// Create log manager
	logManager, err := utils.NewLogManager("./logs")
	if err != nil {
		return nil, fmt.Errorf("failed to create log manager: %w", err)
	}
	orchestrator.LogManager = logManager

	// Initialize ColorManager
	orchestrator.ColorManager = utils.NewColorManager(config.Settings)

	// Initialize RuleRunners
	for _, rule := range config.Rules {
		runner := NewRuleRunner(rule, orchestrator)
		orchestrator.ruleRunners[rule.Name] = runner
	}

	// Connect to gateway if address provided
	/*
		if gatewayAddr != "" {
			if err := orchestrator.connectToGateway(gatewayAddr); err != nil {
				return nil, fmt.Errorf("failed to connect to gateway: %w", err)
			}
		}
	*/

	return orchestrator, nil
}

// GetConfig returns the orchestrator's configuration.
func (o *Orchestrator) GetConfig() *pb.Config {
	return o.Config
}

// Start begins file watching and initializes all RuleRunners
func (o *Orchestrator) Start() error {
	// Start all RuleRunners
	for name, runner := range o.ruleRunners {
		if err := runner.Start(); err != nil {
			return fmt.Errorf("failed to start rule %q: %w", name, err)
		}
	}

	// Setup file watching
	projectRoot := o.ProjectRoot()
	if o.Verbose {
		o.logDevloop("Starting file watcher from project root: %s", projectRoot)
	}

	err := filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Use pattern-based logic to determine if directory should be watched
			if o.shouldWatchDirectory(path) {
				if err := o.Watcher.Add(path); err != nil {
					utils.LogDevloop("Error watching %s: %v", path, err)
				} else if o.Verbose {
					utils.LogDevloop("Watching directory: %s", path)
				}
			} else {
				// Skip this directory and its subdirectories
				if o.Verbose {
					utils.LogDevloop("Skipping directory: %s", path)
				}
				return filepath.SkipDir
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to setup file watcher: %w", err)
	}

	// Start file watching goroutine
	go o.watchFiles()

	// Wait for shutdown
	<-o.done
	return nil
}

// watchFiles monitors for file changes and triggers appropriate RuleRunners
func (o *Orchestrator) watchFiles() {
	for {
		select {
		case event, ok := <-o.Watcher.Events:
			if !ok {
				return
			}

			if o.Verbose {
				o.logDevloop("File event: %s on %s", event.Op, event.Name)
			}

			// Handle directory creation - add new directories to watcher
			if event.Op&fsnotify.Create == fsnotify.Create {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					// Check if this directory should be watched based on user patterns
					shouldWatch := o.shouldWatchDirectory(event.Name)
					if shouldWatch {
						if err := o.Watcher.Add(event.Name); err != nil {
							utils.LogDevloop("Error watching new directory %s: %v", event.Name, err)
						} else if o.Verbose {
							o.logDevloop("Started watching new directory: %s", event.Name)
						}
					}
				}
			}

			// Handle directory removal - remove from watcher
			if event.Op&fsnotify.Remove == fsnotify.Remove {
				// Try to remove from watcher (will silently fail if not being watched)
				o.Watcher.Remove(event.Name)
				if o.Verbose {
					o.logDevloop("Stopped watching removed path: %s", event.Name)
				}
			}

			// Check which rules match this file
			o.runnersMutex.RLock()
			for _, runner := range o.ruleRunners {
				rule := runner.GetRule()
				if matcher := RuleMatches(rule, event.Name, o.ConfigPath); matcher != nil {
					// Pattern matched - check action
					if matcher.Action == "include" {
						if o.Verbose {
							o.logDevloop("Rule %q matched (included) for file %s", rule.Name, event.Name)
						}
						runner.TriggerDebounced()
					} else if matcher.Action == "exclude" {
						if o.Verbose {
							o.logDevloop("Rule %q matched (excluded) for file %s", rule.Name, event.Name)
						}
						// Don't trigger for excluded files
					}
				} else {
					// No patterns matched - check default behavior
					if o.shouldTriggerByDefault(rule) {
						if o.Verbose {
							o.logDevloop("Rule %q matched (default) for file %s", rule.Name, event.Name)
						}
						runner.TriggerDebounced()
					}
				}
			}
			o.runnersMutex.RUnlock()

		case err, ok := <-o.Watcher.Errors:
			if !ok {
				return
			}
			utils.LogDevloop("Watcher error: %v", err)

		case <-o.done:
			return
		}
	}
}

// shouldTriggerByDefault determines if a rule should trigger when no patterns match
func (o *Orchestrator) shouldTriggerByDefault(rule *pb.Rule) bool {
	// Check rule-specific default first
	if rule.DefaultAction != "" {
		return rule.DefaultAction == "include"
	}
	// Fall back to global default
	return o.Config.Settings.DefaultWatchAction == "include"
}

// shouldWatchDirectory determines if a directory should be watched based on user patterns
func (o *Orchestrator) shouldWatchDirectory(dirPath string) bool {
	// Default exclusion patterns (can be overridden by user patterns)
	defaultExclusions := []string{
		"**/node_modules/**",
		"**/vendor/**",
		"**/.*/**", // hidden directories
	}

	// Check if any user pattern would potentially match files in this directory
	o.runnersMutex.RLock()
	defer o.runnersMutex.RUnlock()

	for _, rule := range o.Config.Rules {
		for _, matcher := range rule.Watch {
			for _, pattern := range matcher.Patterns {
				// Resolve pattern relative to rule's work_dir
				resolvedPattern := resolvePattern(pattern, rule, o.ConfigPath)

				// Check if this pattern could match files in the directory
				if o.patternCouldMatchInDirectory(resolvedPattern, dirPath) {
					return true
				}
			}
		}
	}

	// If no user patterns would match, check against default exclusions
	for _, exclusion := range defaultExclusions {
		if matched, _ := doublestar.Match(exclusion, dirPath); matched {
			return false
		}
	}

	// Default to watching if no exclusions match
	return true
}

// patternCouldMatchInDirectory checks if a pattern could potentially match files in a directory
func (o *Orchestrator) patternCouldMatchInDirectory(pattern, dirPath string) bool {
	// If pattern is exact match to directory, it should be watched
	if pattern == dirPath {
		return true
	}

	// If pattern contains the directory as a prefix, it could match files inside
	if strings.HasPrefix(pattern, dirPath+string(filepath.Separator)) {
		return true
	}

	// If directory is a prefix of pattern, it could match files inside
	if strings.HasPrefix(dirPath, filepath.Dir(pattern)) {
		return true
	}

	// Use glob matching to check if pattern could match files in directory
	testFile := filepath.Join(dirPath, "test.txt")
	if matched, _ := doublestar.Match(pattern, testFile); matched {
		return true
	}

	// Check with a few common file extensions
	extensions := []string{".go", ".js", ".ts", ".py", ".java", ".cpp", ".c", ".h"}
	for _, ext := range extensions {
		testFile := filepath.Join(dirPath, "test"+ext)
		if matched, _ := doublestar.Match(pattern, testFile); matched {
			return true
		}
	}

	return false
}

// getDebounceDelayForRule returns the effective debounce delay for a rule
func (o *Orchestrator) getDebounceDelayForRule(rule *pb.Rule) time.Duration {
	// Rule-specific delay takes precedence
	if rule.DebounceDelay != nil {
		return time.Millisecond * time.Duration(*rule.DebounceDelay)
	}
	// Fall back to global default
	if o.Config.Settings.DefaultDebounceDelay != nil {
		return time.Duration(*o.Config.Settings.DefaultDebounceDelay) * time.Millisecond
	}
	// Final fallback to hardcoded default
	return 500 * time.Millisecond
}

// isVerboseForRule returns whether verbose logging is enabled for a rule
func (o *Orchestrator) isVerboseForRule(rule *pb.Rule) bool {
	// Rule-specific setting takes precedence
	if rule.Verbose != nil {
		return *rule.Verbose
	}
	// Fall back to global setting
	return o.Config.Settings.Verbose
}

// safeDone closes the done channel safely using sync.Once
func (o *Orchestrator) safeDone() {
	o.doneOnce.Do(func() {
		close(o.done)
	})
}

// Stop gracefully shuts down the orchestrator
func (o *Orchestrator) Stop() error {
	utils.LogDevloop("Stopping orchestrator...")
	o.safeDone()

	// Stop all RuleRunners
	var wg sync.WaitGroup
	o.runnersMutex.RLock()
	for name, runner := range o.ruleRunners {
		wg.Add(1)
		go func(n string, r *RuleRunner) {
			defer wg.Done()
			if err := r.Stop(); err != nil {
				utils.LogDevloop("Error stopping rule %q: %v", n, err)
			}
		}(name, runner)
	}
	o.runnersMutex.RUnlock()

	wg.Wait()
	utils.LogDevloop("All rules stopped")

	// Disconnect from gateway
	// if o.gatewayStream != nil { o.disconnectFromGateway() }

	// Close log manager
	if err := o.LogManager.Close(); err != nil {
		utils.LogDevloop("Error closing log manager: %v", err)
	}

	return o.Watcher.Close()
}

// GetRuleStatus returns the status of a specific rule
func (o *Orchestrator) GetRuleStatus(ruleName string) (rule *pb.Rule, status *pb.RuleStatus, ok bool) {
	o.runnersMutex.RLock()
	defer o.runnersMutex.RUnlock()

	if runner, found := o.ruleRunners[ruleName]; found {
		rule = runner.rule
		status = runner.GetStatus()
		ok = true
	}
	return
}

// ProjectRoot returns the project root directory
func (o *Orchestrator) ProjectRoot() string {
	return filepath.Dir(o.ConfigPath)
}

// GetWatchedPaths returns a unique list of all paths being watched by any rule
func (o *Orchestrator) GetWatchedPaths() []string {
	o.runnersMutex.RLock()
	defer o.runnersMutex.RUnlock()

	watchedPaths := make(map[string]struct{})
	for _, rule := range o.Config.Rules {
		for _, matcher := range rule.Watch {
			for _, pattern := range matcher.Patterns {
				watchedPaths[pattern] = struct{}{}
			}
		}
	}

	paths := make([]string, 0, len(watchedPaths))
	for path := range watchedPaths {
		paths = append(paths, path)
	}
	return paths
}

// ReadFileContent reads and returns the content of a specified file
func (o *Orchestrator) ReadFileContent(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// StreamLogs streams the logs for a given rule to the provided Writer
func (o *Orchestrator) StreamLogs(ruleName string, filter string, timeoutSeconds int64, writer *gocurrent.Writer[*pb.StreamLogsResponse]) error {
	// Check if rule exists
	o.runnersMutex.RLock()
	_, ruleExists := o.ruleRunners[ruleName]
	o.runnersMutex.RUnlock()

	if !ruleExists {
		return fmt.Errorf("rule %q not found", ruleName)
	}

	// Check if log file exists
	logFilePath := filepath.Join("./logs", fmt.Sprintf("%s.log", ruleName))
	_, err := os.Stat(logFilePath)
	logFileExists := err == nil

	// Case 1: Log file exists - delegate to LogManager
	if logFileExists {
		return o.LogManager.StreamLogs(ruleName, filter, timeoutSeconds, writer)
	}

	// Case 2: Log file doesn't exist - rule hasn't started yet
	// Send waiting message and wait for rule to start with timeout
	initialMsg := &pb.StreamLogsResponse{
		Lines: []*pb.LogLine{
			{
				RuleName:  ruleName,
				Line:      fmt.Sprintf("Waiting for rule '%s' to start...", ruleName),
				Timestamp: time.Now().UnixMilli(),
			},
		},
	}
	if !writer.Send(initialMsg) {
		return fmt.Errorf("failed to send initial message")
	}

	// Wait for log file to appear with timeout
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			timeoutMsg := &pb.StreamLogsResponse{
				Lines: []*pb.LogLine{
					{
						RuleName:  ruleName,
						Line:      fmt.Sprintf("Timeout waiting for rule '%s' to start", ruleName),
						Timestamp: time.Now().UnixMilli(),
					},
				},
			}
			writer.Send(timeoutMsg)
			return fmt.Errorf("timeout waiting for rule %q to start", ruleName)

		case <-ticker.C:
			_, err := os.Stat(logFilePath)
			if err == nil {
				// Log file now exists - delegate to LogManager for streaming
				return o.LogManager.StreamLogs(ruleName, filter, timeoutSeconds, writer)
			}
		}
	}
}

// TriggerRule manually triggers the execution of a specific rule
func (o *Orchestrator) TriggerRule(ruleName string) error {
	o.runnersMutex.RLock()
	defer o.runnersMutex.RUnlock()

	runner, exists := o.ruleRunners[ruleName]
	if !exists {
		return fmt.Errorf("rule %q not found", ruleName)
	}

	// Trigger the rule runner's debounced execution
	runner.TriggerDebounced()
	return nil
}

// SetGlobalDebounceDelay sets the default debounce delay for all rules
func (o *Orchestrator) SetGlobalDebounceDelay(duration time.Duration) {
	o.runnersMutex.Lock()
	defer o.runnersMutex.Unlock()

	// Update the global setting
	dur := uint64(duration / time.Millisecond)
	o.Config.Settings.DefaultDebounceDelay = &dur

	// Update all existing rule runners
	for _, runner := range o.ruleRunners {
		// Only update if the rule doesn't have its own setting
		rule := runner.GetRule()
		if rule.DebounceDelay == nil {
			runner.SetDebounceDelay(duration)
		}
	}
}

// SetRuleDebounceDelay sets the debounce delay for a specific rule
func (o *Orchestrator) SetRuleDebounceDelay(ruleName string, duration time.Duration) error {
	o.runnersMutex.Lock()
	defer o.runnersMutex.Unlock()

	// Find the rule and update its debounce delay
	for i := range o.Config.Rules {
		if o.Config.Rules[i].Name == ruleName {
			dur := uint64(duration / time.Millisecond)
			o.Config.Rules[i].DebounceDelay = &dur

			// Update the rule runner if it exists
			if runner, exists := o.ruleRunners[ruleName]; exists {
				runner.SetDebounceDelay(duration)
			}
			return nil
		}
	}
	return fmt.Errorf("rule %q not found", ruleName)
}

// SetVerbose sets the global verbose flag
func (o *Orchestrator) SetVerbose(verbose bool) {
	o.runnersMutex.Lock()
	defer o.runnersMutex.Unlock()

	o.Config.Settings.Verbose = verbose

	// Update all existing rule runners
	for _, runner := range o.ruleRunners {
		// Only update if the rule doesn't have its own setting
		rule := runner.GetRule()
		if rule.Verbose == nil {
			runner.SetVerbose(verbose)
		}
	}
}

// SetRuleVerbose sets the verbose flag for a specific rule
func (o *Orchestrator) SetRuleVerbose(ruleName string, verbose bool) error {
	o.runnersMutex.Lock()
	defer o.runnersMutex.Unlock()

	// Find the rule and update its verbose flag
	for i := range o.Config.Rules {
		if o.Config.Rules[i].Name == ruleName {
			o.Config.Rules[i].Verbose = &verbose

			// Update the rule runner if it exists
			if runner, exists := o.ruleRunners[ruleName]; exists {
				runner.SetVerbose(verbose)
			}
			return nil
		}
	}
	return fmt.Errorf("rule %q not found", ruleName)
}

// logDevloop logs devloop internal messages with consistent formatting
func (o *Orchestrator) logDevloop(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)

	if o.Config.Settings.PrefixLogs && o.Config.Settings.PrefixMaxLength > 0 {
		// Format with left-aligned "devloop" prefix to match rule output format
		prefix := "devloop"
		totalPadding := int(o.Config.Settings.PrefixMaxLength - uint32(len(prefix)))
		leftAlignedPrefix := prefix + strings.Repeat(" ", totalPadding)
		prefixStr := "[" + leftAlignedPrefix + "] "

		// Add color if enabled
		if o.ColorManager != nil && o.ColorManager.IsEnabled() {
			// Create a fake rule for devloop messages to get consistent coloring
			devloopRule := &pb.Rule{Name: "devloop"}
			coloredPrefix := o.ColorManager.FormatPrefix(prefixStr, devloopRule)
			fmt.Printf("%s%s\n", coloredPrefix, message)
		} else {
			fmt.Printf("%s%s\n", prefixStr, message)
		}
	} else {
		// Standard log format but with devloop color if available
		if o.ColorManager != nil && o.ColorManager.IsEnabled() {
			utils.LogDevloop("%s", message)
		} else {
			utils.LogDevloop("%s", message)
		}
	}
}

//// Gateway related things
/*
func (o *Orchestrator) handleGatewayStreamSend() {
	for {
		select {
		case <-o.done:
			return
		case msg := <-o.gatewaySendChan:
			if err := o.gatewayStream.Send(msg); err != nil {
				utils.LogDevloop("Error sending message to gateway: %v", err)
				o.safeDone()
				return
			}
		}
	}
}
*/

// connectToGateway establishes connection to the gateway service
/*
func (o *Orchestrator) connectToGateway(gatewayAddr string) error {
		conn, err := grpc.NewClient(gatewayAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("failed to connect to gateway %q: %w", gatewayAddr, err)
		}

		o.gatewayClient = pb.NewDevloopGatewayServiceClient(conn)

		// Establish bidirectional stream
		stream, err := o.gatewayClient.Communicate(context.Background())
		if err != nil {
			return fmt.Errorf("failed to open gateway communication stream: %w", err)
		}
		o.gatewayStream = stream

		// Send registration
		registerMsg := &pb.DevloopMessage{
			Content: &pb.DevloopMessage_RegisterRequest{
				RegisterRequest: &pb.RegisterRequest{
					ProjectInfo: &pb.ProjectInfo{
						ProjectId:   o.projectID,
						ProjectRoot: o.ProjectRoot(),
					},
				},
			},
		}

		if err := o.gatewayStream.Send(registerMsg); err != nil {
			return fmt.Errorf("failed to send registration to gateway: %w", err)
		}

		utils.LogDevloop("Registered with gateway as project %q", o.projectID)

		// Start gateway communication handlers
		go o.handleGatewayStreamRecv()
		go o.handleGatewayStreamSend()

	return nil
}

// disconnectFromGateway cleanly disconnects from the gateway
func (o *Orchestrator) disconnectFromGateway() {
		unregisterMsg := &pb.DevloopMessage{
			Content: &pb.DevloopMessage_UnregisterRequest{
				UnregisterRequest: &pb.UnregisterRequest{
					ProjectId: o.projectID,
				},
			},
		}

		if err := o.gatewayStream.Send(unregisterMsg); err != nil {
			utils.LogDevloop("Error sending unregister message to gateway: %v", err)
		}

		o.gatewayStream.CloseSend()
}

// handleGatewayStreamRecv handles incoming messages from gateway
func (o *Orchestrator) handleGatewayStreamRecv() {
	utils.LogDevloop("Starting gateway stream receiver.")
	for {
		select {
		case <-o.done:
			utils.LogDevloop("Gateway stream receiver stopping.")
			return
		default:
			msg, err := o.gatewayStream.Recv()
			if err == io.EOF {
				utils.LogDevloop("Gateway closed stream (EOF). Shutting down.")
				o.safeDone()
				return
			}
			if err != nil {
				utils.LogDevloop("Error receiving from gateway stream: %v. Shutting down.", err)
				o.safeDone()
				return
			}

			switch content := msg.GetContent().(type) {
			case *pb.DevloopMessage_TriggerRuleRequest:
				go o.handleTriggerRuleRequest(msg.GetCorrelationId(), content.TriggerRuleRequest)
			case *pb.DevloopMessage_GetConfigRequest:
				go o.handleGetConfigRequest(msg.GetCorrelationId(), content.GetConfigRequest)
			case *pb.DevloopMessage_GetRuleStatusRequest:
				go o.handleGetRuleStatusRequest(msg.GetCorrelationId(), content.GetRuleStatusRequest)
			case *pb.DevloopMessage_ListWatchedPathsRequest:
				go o.handleListWatchedPathsRequest(msg.GetCorrelationId(), content.ListWatchedPathsRequest)
			case *pb.DevloopMessage_ReadFileContentRequest:
				go o.handleReadFileContentRequest(msg.GetCorrelationId(), content.ReadFileContentRequest)
			case *pb.DevloopMessage_GetHistoricalLogsRequest:
				go o.handleGetHistoricalLogsRequest(msg.GetCorrelationId(), content.GetHistoricalLogsRequest)
			// Handle responses to devloop-initiated requests
			case *pb.DevloopMessage_RegisterRequest:
				utils.LogDevloop("Received unexpected RegisterRequest from pb.")
			case *pb.DevloopMessage_UnregisterRequest:
				utils.LogDevloop("Received unexpected UnregisterRequest from pb.")
			case *pb.DevloopMessage_LogLine:
				utils.LogDevloop("Received unexpected LogLine from pb.")
			case *pb.DevloopMessage_UpdateRuleStatusRequest:
				utils.LogDevloop("Received unexpected UpdateRuleStatusRequest from pb.")
			default:
				// This is a response to a devloop-initiated request
				if msg.GetCorrelationId() != "" {
					o.runnersMutex.RLock()
					respChan, ok := o.responseChan[msg.GetCorrelationId()]
					o.runnersMutex.RUnlock()
					if ok {
						select {
						case respChan <- msg:
						default:
							utils.LogDevloop("Response channel for correlation ID %s is full, dropping response.", msg.GetCorrelationId())
						}
					} else {
						utils.LogDevloop("Received response for unknown correlation ID %s: %T", msg.GetCorrelationId(), content)
					}
				}
			}
		}
	}
}

// handleGatewayStreamSend sends outgoing messages to gateway
*/
