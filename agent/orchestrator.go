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
	"github.com/panyam/gocurrent"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"github.com/panyam/devloop/utils"
)

// TriggerChain tracks cross-rule trigger relationships for cycle detection
type TriggerChain struct {
	chains map[string][]string // ruleName -> list of rules it triggered
	mutex  sync.RWMutex
}

// NewTriggerChain creates a new trigger chain tracker
func NewTriggerChain() *TriggerChain {
	return &TriggerChain{
		chains: make(map[string][]string),
	}
}

// RecordTrigger records that sourceRule triggered targetRule
func (tc *TriggerChain) RecordTrigger(sourceRule, targetRule string) {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	if tc.chains[sourceRule] == nil {
		tc.chains[sourceRule] = make([]string, 0)
	}

	// Add to chain if not already present
	for _, existing := range tc.chains[sourceRule] {
		if existing == targetRule {
			return
		}
	}

	tc.chains[sourceRule] = append(tc.chains[sourceRule], targetRule)
}

// DetectCycle detects if adding a new trigger would create a cycle
func (tc *TriggerChain) DetectCycle(sourceRule, targetRule string, maxDepth uint32) bool {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	visited := make(map[string]bool)
	return tc.detectCycleRecursive(targetRule, sourceRule, visited, 0, maxDepth)
}

// detectCycleRecursive performs recursive cycle detection
func (tc *TriggerChain) detectCycleRecursive(currentRule, targetRule string, visited map[string]bool, depth uint32, maxDepth uint32) bool {
	if depth > maxDepth {
		return true // Consider max depth exceeded as a cycle
	}

	if currentRule == targetRule {
		return true // Cycle detected
	}

	if visited[currentRule] {
		return false // Already visited this path
	}

	visited[currentRule] = true

	// Check all rules that currentRule triggers
	for _, triggeredRule := range tc.chains[currentRule] {
		if tc.detectCycleRecursive(triggeredRule, targetRule, visited, depth+1, maxDepth) {
			return true
		}
	}

	return false
}

// GetChainLength returns the length of the trigger chain starting from a rule
func (tc *TriggerChain) GetChainLength(ruleName string) int {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	visited := make(map[string]bool)
	return tc.getChainLengthRecursive(ruleName, visited, 0)
}

// getChainLengthRecursive calculates chain length recursively
func (tc *TriggerChain) getChainLengthRecursive(ruleName string, visited map[string]bool, depth int) int {
	if visited[ruleName] {
		return depth // Cycle detected, return current depth
	}

	visited[ruleName] = true
	maxDepth := depth

	for _, triggeredRule := range tc.chains[ruleName] {
		childDepth := tc.getChainLengthRecursive(triggeredRule, visited, depth+1)
		if childDepth > maxDepth {
			maxDepth = childDepth
		}
	}

	return maxDepth
}

// CleanupOldChains removes old trigger chains to prevent memory growth
func (tc *TriggerChain) CleanupOldChains() {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	// For now, just clear all chains periodically
	// In a more sophisticated implementation, we could track timestamps
	tc.chains = make(map[string][]string)
}

// FileModificationTracker tracks file modification frequencies for thrashing detection
type FileModificationTracker struct {
	modifications map[string][]time.Time // filePath -> modification timestamps
	mutex         sync.RWMutex
}

// NewFileModificationTracker creates a new file modification tracker
func NewFileModificationTracker() *FileModificationTracker {
	return &FileModificationTracker{
		modifications: make(map[string][]time.Time),
	}
}

// RecordModification records a file modification
func (fmt *FileModificationTracker) RecordModification(filePath string) {
	fmt.mutex.Lock()
	defer fmt.mutex.Unlock()

	if fmt.modifications[filePath] == nil {
		fmt.modifications[filePath] = make([]time.Time, 0)
	}

	fmt.modifications[filePath] = append(fmt.modifications[filePath], time.Now())
}

// IsFileThrashing checks if a file is being modified too frequently
func (fmt *FileModificationTracker) IsFileThrashing(filePath string, windowSeconds, threshold uint32) bool {
	fmt.mutex.RLock()
	defer fmt.mutex.RUnlock()

	modifications := fmt.modifications[filePath]
	if modifications == nil {
		return false
	}

	cutoff := time.Now().Add(-time.Duration(windowSeconds) * time.Second)
	count := 0

	for _, modTime := range modifications {
		if modTime.After(cutoff) {
			count++
		}
	}

	return uint32(count) > threshold
}

// GetModificationCount returns the number of modifications within the time window
func (fmt *FileModificationTracker) GetModificationCount(filePath string, windowSeconds uint32) int {
	fmt.mutex.RLock()
	defer fmt.mutex.RUnlock()

	modifications := fmt.modifications[filePath]
	if modifications == nil {
		return 0
	}

	cutoff := time.Now().Add(-time.Duration(windowSeconds) * time.Second)
	count := 0

	for _, modTime := range modifications {
		if modTime.After(cutoff) {
			count++
		}
	}

	return count
}

// CleanupOldModifications removes old modification records
func (fmt *FileModificationTracker) CleanupOldModifications(maxAge time.Duration) {
	fmt.mutex.Lock()
	defer fmt.mutex.Unlock()

	cutoff := time.Now().Add(-maxAge)

	for filePath, modifications := range fmt.modifications {
		validMods := make([]time.Time, 0)

		for _, modTime := range modifications {
			if modTime.After(cutoff) {
				validMods = append(validMods, modTime)
			}
		}

		if len(validMods) == 0 {
			delete(fmt.modifications, filePath)
		} else {
			fmt.modifications[filePath] = validMods
		}
	}
}

// GetThrashingFiles returns a list of files that are currently thrashing
func (fmt *FileModificationTracker) GetThrashingFiles(windowSeconds, threshold uint32) []string {
	fmt.mutex.RLock()
	defer fmt.mutex.RUnlock()

	thrashingFiles := make([]string, 0)
	cutoff := time.Now().Add(-time.Duration(windowSeconds) * time.Second)

	for filePath, modifications := range fmt.modifications {
		count := 0
		for _, modTime := range modifications {
			if modTime.After(cutoff) {
				count++
			}
		}

		if uint32(count) > threshold {
			thrashingFiles = append(thrashingFiles, filePath)
		}
	}

	return thrashingFiles
}

// CycleBreaker manages advanced cycle breaking and resolution mechanisms
type CycleBreaker struct {
	disabledRules   map[string]time.Time // ruleName -> time when disabled
	disabledMutex   sync.RWMutex
	emergencyBreaks map[string]int // ruleName -> number of emergency breaks
	emergencyMutex  sync.RWMutex
}

// NewCycleBreaker creates a new cycle breaker
func NewCycleBreaker() *CycleBreaker {
	return &CycleBreaker{
		disabledRules:   make(map[string]time.Time),
		emergencyBreaks: make(map[string]int),
	}
}

// DisableRule temporarily disables a rule for cycle breaking
func (cb *CycleBreaker) DisableRule(ruleName string, duration time.Duration) {
	cb.disabledMutex.Lock()
	defer cb.disabledMutex.Unlock()

	cb.disabledRules[ruleName] = time.Now().Add(duration)
}

// IsRuleDisabled checks if a rule is currently disabled
func (cb *CycleBreaker) IsRuleDisabled(ruleName string) bool {
	cb.disabledMutex.RLock()
	defer cb.disabledMutex.RUnlock()

	disabledUntil, exists := cb.disabledRules[ruleName]
	if !exists {
		return false
	}

	if time.Now().After(disabledUntil) {
		// Rule is no longer disabled, clean up
		delete(cb.disabledRules, ruleName)
		return false
	}

	return true
}

// TriggerEmergencyBreak triggers an emergency cycle break for a rule
func (cb *CycleBreaker) TriggerEmergencyBreak(ruleName string) {
	cb.emergencyMutex.Lock()
	defer cb.emergencyMutex.Unlock()

	cb.emergencyBreaks[ruleName]++

	// Disable rule for progressively longer periods
	disableDuration := time.Duration(cb.emergencyBreaks[ruleName]) * time.Minute
	if disableDuration > 30*time.Minute {
		disableDuration = 30 * time.Minute
	}

	cb.disabledMutex.Lock()
	cb.disabledRules[ruleName] = time.Now().Add(disableDuration)
	cb.disabledMutex.Unlock()
}

// GetEmergencyBreakCount returns the number of emergency breaks for a rule
func (cb *CycleBreaker) GetEmergencyBreakCount(ruleName string) int {
	cb.emergencyMutex.RLock()
	defer cb.emergencyMutex.RUnlock()

	return cb.emergencyBreaks[ruleName]
}

// GetDisabledRules returns a map of currently disabled rules and their disable times
func (cb *CycleBreaker) GetDisabledRules() map[string]time.Time {
	cb.disabledMutex.RLock()
	defer cb.disabledMutex.RUnlock()

	result := make(map[string]time.Time)
	for ruleName, disabledUntil := range cb.disabledRules {
		if time.Now().Before(disabledUntil) {
			result[ruleName] = disabledUntil
		}
	}

	return result
}

// GenerateCycleResolutionSuggestions generates suggestions for resolving detected cycles
func (cb *CycleBreaker) GenerateCycleResolutionSuggestions(ruleName string, cycleType string) []string {
	suggestions := make([]string, 0)

	switch cycleType {
	case "self-referential":
		suggestions = append(suggestions, fmt.Sprintf("Consider excluding output files from rule %q patterns", ruleName))
		suggestions = append(suggestions, fmt.Sprintf("Add 'cycle_protection: false' to rule %q if intentional", ruleName))
		suggestions = append(suggestions, fmt.Sprintf("Use a different working directory for rule %q outputs", ruleName))
	case "cross-rule":
		suggestions = append(suggestions, fmt.Sprintf("Review file patterns for rule %q to avoid triggering cycles", ruleName))
		suggestions = append(suggestions, "Add exclusion patterns to break the cycle chain")
		suggestions = append(suggestions, "Consider using different working directories for related rules")
	case "file-thrashing":
		suggestions = append(suggestions, fmt.Sprintf("Rule %q is causing file thrashing - consider increasing debounce delay", ruleName))
		suggestions = append(suggestions, "Add file exclusion patterns to avoid watching frequently modified files")
		suggestions = append(suggestions, fmt.Sprintf("Review command output patterns for rule %q", ruleName))
	default:
		suggestions = append(suggestions, fmt.Sprintf("Review rule %q configuration for potential cycle causes", ruleName))
		suggestions = append(suggestions, "Consider enabling verbose logging to identify the cycle source")
	}

	return suggestions
}

// CleanupOldData removes old emergency break and disable records
func (cb *CycleBreaker) CleanupOldData() {
	cb.emergencyMutex.Lock()
	defer cb.emergencyMutex.Unlock()

	cb.disabledMutex.Lock()
	defer cb.disabledMutex.Unlock()

	// Clean up old disabled rules
	for ruleName, disabledUntil := range cb.disabledRules {
		if time.Now().After(disabledUntil.Add(time.Hour)) { // Keep records for 1 hour after expiry
			delete(cb.disabledRules, ruleName)
		}
	}

	// Reset emergency break counts after 24 hours of no issues
	// (In a real implementation, this would be more sophisticated)
	cb.emergencyBreaks = make(map[string]int)
}

// Orchestrator manages file watching and delegates execution to RuleRunners
type Orchestrator struct {
	projectID    string
	ConfigPath   string
	Config       *pb.Config
	Verbose      bool
	LogManager   *utils.LogManager
	ColorManager *utils.ColorManager

	// Rule management
	ruleRunners  map[string]*RuleRunner
	runnersMutex sync.RWMutex

	// Trigger chain tracking for cross-rule cycle detection
	triggerChain *TriggerChain

	// File modification tracking for thrashing detection
	fileModTracker *FileModificationTracker

	// Advanced cycle breaking and resolution
	cycleBreaker *CycleBreaker

	// Currently executing rule context for trigger chain tracking
	currentExecutingRule string
	executionMutex       sync.RWMutex

	// Control channels
	done            chan bool
	doneOnce        sync.Once
	criticalFailure chan string // Channel for critical rule failures
	
	// Rule execution semaphore for controlling parallelism
	ruleSemaphore chan struct{} // Buffered channel acting as semaphore

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
		ConfigPath:       absConfigPath,
		Config:           config,
		projectID:        projectID,
		done:             make(chan bool),
		criticalFailure:  make(chan string, 10), // Buffered channel for critical failures
		ruleRunners:      make(map[string]*RuleRunner),
		triggerChain:     NewTriggerChain(),
		fileModTracker:   NewFileModificationTracker(),
		cycleBreaker:     NewCycleBreaker(),
		debounceDuration: 500 * time.Millisecond,
	}
	
	// Initialize rule execution semaphore based on max_parallel_rules setting
	maxParallel := orchestrator.getMaxParallelRules()
	if maxParallel > 0 {
		orchestrator.ruleSemaphore = make(chan struct{}, maxParallel)
		utils.LogDevloop("Initialized rule execution semaphore with max parallel rules: %d", maxParallel)
	} else {
		// No limit - semaphore will be nil and won't be used
		orchestrator.ruleSemaphore = nil
		utils.LogDevloop("Rule execution semaphore disabled (unlimited parallel rules)")
	}

	// Validate configuration for potential cycles
	if err := orchestrator.ValidateConfig(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
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
	// Start all RuleRunners (now non-blocking - rules initialize in background)
	for name, runner := range o.ruleRunners {
		if err := runner.Start(); err != nil {
			// Only immediate validation failures would cause Start() to return error
			utils.LogDevloop("Failed to start rule %q: %v", name, err)
			return fmt.Errorf("rule validation failed for %q: %w", name, err)
		}
	}

	utils.LogDevloop("All rules started - initialization retry logic running in background")

	// Start trigger history cleanup goroutine
	go o.cleanupTriggerHistory()

	// Wait for shutdown or critical failure
	select {
	case <-o.done:
		return nil
	case failedRule := <-o.criticalFailure:
		return fmt.Errorf("devloop exiting due to critical failure in rule %q", failedRule)
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



// getMaxParallelRules returns the effective max parallel rules setting
func (o *Orchestrator) getMaxParallelRules() uint32 {
	if o.Config.Settings.MaxParallelRules != 0 {
		return o.Config.Settings.MaxParallelRules
	}
	return 0 // Default: no limit
}

// getSemaphoreStatus returns information about the current semaphore state
func (o *Orchestrator) getSemaphoreStatus() (active int, capacity int, unlimited bool) {
	if o.ruleSemaphore == nil {
		return 0, 0, true
	}
	return len(o.ruleSemaphore), cap(o.ruleSemaphore), false
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

	return nil
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

	// Trigger the rule runner's debounced execution (bypass rate limiting for manual triggers)
	runner.TriggerDebouncedWithOptions(true)
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

// ValidateConfig performs static cycle detection and validation on the loaded configuration
func (o *Orchestrator) ValidateConfig() error {
	// Check if cycle detection is enabled
	if !o.isCycleDetectionEnabled() {
		return nil
	}

	// Check if static validation is enabled
	if !o.isStaticValidationEnabled() {
		return nil
	}

	if err := o.detectStaticCycles(); err != nil {
		return fmt.Errorf("static cycle detected: %w", err)
	}
	return nil
}

// detectStaticCycles performs static analysis to detect potential infinite loops
func (o *Orchestrator) detectStaticCycles() error {
	for _, rule := range o.Config.Rules {
		if err := o.validateRuleForSelfReference(rule); err != nil {
			return err
		}
	}
	return nil
}

// validateRuleForSelfReference checks if a rule might trigger itself
func (o *Orchestrator) validateRuleForSelfReference(rule *pb.Rule) error {
	// Check if cycle protection is enabled for this specific rule
	if !o.isRuleCycleProtectionEnabled(rule) {
		return nil
	}

	// Get the rule's working directory
	workdir := o.getRuleWorkdir(rule)

	// Check each watch pattern against the working directory
	// Only consider "include" patterns for cycle detection, as "exclude" patterns prevent triggering
	for _, matcher := range rule.Watch {
		if matcher.Action != "include" {
			continue // Skip exclude patterns for cycle detection
		}
		for _, pattern := range matcher.Patterns {
			// Resolve pattern relative to rule's work_dir
			resolvedPattern := resolvePattern(pattern, rule, o.ConfigPath)

			// Check if the pattern could watch files in the rule's working directory
			if o.pathOverlaps(resolvedPattern, workdir) {
				utils.LogDevloop("Warning: Rule %q may trigger itself - pattern %q watches workdir %q",
					rule.Name, pattern, workdir)
				// For now, just warn - don't error out to maintain backward compatibility
			}
		}
	}
	return nil
}

// getRuleWorkdir returns the effective working directory for a rule
func (o *Orchestrator) getRuleWorkdir(rule *pb.Rule) string {
	if rule.WorkDir != "" {
		if filepath.IsAbs(rule.WorkDir) {
			return rule.WorkDir
		}
		// Relative to config file location
		return filepath.Join(filepath.Dir(o.ConfigPath), rule.WorkDir)
	}
	// Default to config file directory
	return filepath.Dir(o.ConfigPath)
}

// pathOverlaps checks if a pattern could potentially match files in a directory
func (o *Orchestrator) pathOverlaps(pattern, dirPath string) bool {
	// Make paths comparable by cleaning them
	pattern = filepath.Clean(pattern)
	dirPath = filepath.Clean(dirPath)

	// If pattern is exact match to directory, it overlaps
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

	// Check with common file extensions that might be created by commands
	extensions := []string{".go", ".js", ".ts", ".py", ".java", ".cpp", ".c", ".h", ".log", ".txt", ".md"}
	for _, ext := range extensions {
		testFile := filepath.Join(dirPath, "test"+ext)
		if matched, _ := doublestar.Match(pattern, testFile); matched {
			return true
		}
	}

	return false
}

// getCycleDetectionSettings returns the effective cycle detection settings with defaults
func (o *Orchestrator) getCycleDetectionSettings() *pb.CycleDetectionSettings {
	if o.Config.Settings.CycleDetection != nil {
		return o.Config.Settings.CycleDetection
	}

	// Return default settings if not configured
	return &pb.CycleDetectionSettings{
		Enabled:                 true,  // Enable by default
		StaticValidation:        true,  // Enable static validation by default
		DynamicProtection:       false, // Disable dynamic protection by default for now
		MaxTriggersPerMinute:    10,    // Default rate limit
		MaxChainDepth:           5,     // Default chain depth limit
		FileThrashWindowSeconds: 60,    // Default 60 second window
		FileThrashThreshold:     5,     // Default threshold
	}
}

// isCycleDetectionEnabled checks if cycle detection is enabled globally
func (o *Orchestrator) isCycleDetectionEnabled() bool {
	settings := o.getCycleDetectionSettings()
	return settings.Enabled
}

// isStaticValidationEnabled checks if static validation is enabled
func (o *Orchestrator) isStaticValidationEnabled() bool {
	settings := o.getCycleDetectionSettings()
	return settings.StaticValidation
}

// isDynamicProtectionEnabled checks if dynamic protection (rate limiting) is enabled
func (o *Orchestrator) isDynamicProtectionEnabled() bool {
	settings := o.getCycleDetectionSettings()
	return settings.DynamicProtection
}

// isRuleCycleProtectionEnabled checks if cycle protection is enabled for a specific rule
func (o *Orchestrator) isRuleCycleProtectionEnabled(rule *pb.Rule) bool {
	// Check rule-specific override first
	if rule.CycleProtection != nil {
		if o.Verbose {
			utils.LogDevloop("Rule %q has cycle_protection override: %v", rule.Name, *rule.CycleProtection)
		}
		return *rule.CycleProtection
	}

	// Fall back to global setting
	globalEnabled := o.isCycleDetectionEnabled()
	if o.Verbose {
		utils.LogDevloop("Rule %q using global cycle detection setting: %v", rule.Name, globalEnabled)
	}
	return globalEnabled
}

// setCurrentExecutingRule sets the currently executing rule for trigger chain tracking
func (o *Orchestrator) setCurrentExecutingRule(ruleName string) {
	o.executionMutex.Lock()
	defer o.executionMutex.Unlock()
	o.currentExecutingRule = ruleName
}

// getCurrentExecutingRule returns the currently executing rule
func (o *Orchestrator) getCurrentExecutingRule() string {
	o.executionMutex.RLock()
	defer o.executionMutex.RUnlock()
	return o.currentExecutingRule
}

// clearCurrentExecutingRule clears the currently executing rule
func (o *Orchestrator) clearCurrentExecutingRule() {
	o.executionMutex.Lock()
	defer o.executionMutex.Unlock()
	o.currentExecutingRule = ""
}

// cleanupTriggerHistory periodically cleans up old trigger records to prevent memory growth
func (o *Orchestrator) cleanupTriggerHistory() {
	ticker := time.NewTicker(5 * time.Minute) // Cleanup every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-o.done:
			return
		case <-ticker.C:
			o.runnersMutex.RLock()
			for _, runner := range o.ruleRunners {
				runner.CleanupTriggerHistory()
			}
			o.runnersMutex.RUnlock()

			// Cleanup trigger chains
			o.triggerChain.CleanupOldChains()

			// Cleanup file modification tracking
			o.fileModTracker.CleanupOldModifications(10 * time.Minute)

			// Cleanup cycle breaker data
			o.cycleBreaker.CleanupOldData()
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
