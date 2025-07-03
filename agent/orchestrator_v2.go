package agent

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/panyam/devloop/gateway"
	pb "github.com/panyam/devloop/gen/go/protos/devloop/v1"
	"github.com/panyam/devloop/utils"
)

// OrchestratorV2 manages file watching and delegates execution to RuleRunners
type OrchestratorV2 struct {
	ConfigPath   string
	Config       *gateway.Config
	Verbose      bool
	Watcher      *fsnotify.Watcher
	LogManager   *utils.LogManager
	ColorManager *utils.ColorManager

	// Rule management
	ruleRunners  map[string]RuleRunner
	runnersMutex sync.RWMutex

	// Gateway communication
	gatewayClient pb.DevloopGatewayServiceClient
	gatewayStream pb.DevloopGatewayService_CommunicateClient
	projectID     string

	// Control channels
	done     chan bool
	doneOnce sync.Once

	// Gateway communication channels
	gatewaySendChan chan *pb.DevloopMessage
	responseChan    map[string]chan *pb.DevloopMessage

	// Debounce settings
	debounceDuration time.Duration
}

// NewOrchestratorV2 creates a new orchestrator using RuleRunners
func NewOrchestratorV2(configPath string, gatewayAddr string) (*OrchestratorV2, error) {
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
	if config.Settings.ProjectID != "" {
		projectID = config.Settings.ProjectID
	} else {
		// Generate project ID from path
		projectRoot := filepath.Dir(absConfigPath)
		hasher := sha1.New()
		hasher.Write([]byte(projectRoot))
		projectID = hex.EncodeToString(hasher.Sum(nil))[:16]
	}

	orchestrator := &OrchestratorV2{
		ConfigPath:       absConfigPath,
		Config:           config,
		Watcher:          watcher,
		projectID:        projectID,
		done:             make(chan bool),
		ruleRunners:      make(map[string]RuleRunner),
		gatewaySendChan:  make(chan *pb.DevloopMessage, 100),
		responseChan:     make(map[string]chan *pb.DevloopMessage),
		debounceDuration: 500 * time.Millisecond,
	}

	// Create log manager
	logManager, err := utils.NewLogManager("./logs")
	if err != nil {
		return nil, fmt.Errorf("failed to create log manager: %w", err)
	}
	orchestrator.LogManager = logManager

	// Initialize ColorManager
	orchestrator.ColorManager = utils.NewColorManager(&config.Settings)

	// Initialize RuleRunners
	for _, rule := range config.Rules {
		runner := NewRuleRunner(rule, orchestrator)
		orchestrator.ruleRunners[rule.Name] = runner
	}

	// Connect to gateway if address provided
	if gatewayAddr != "" {
		if err := orchestrator.connectToGateway(gatewayAddr); err != nil {
			return nil, fmt.Errorf("failed to connect to gateway: %w", err)
		}
	}

	return orchestrator, nil
}

// GetConfig returns the orchestrator's configuration.
func (o *OrchestratorV2) GetConfig() *gateway.Config {
	return o.Config
}

// Start begins file watching and initializes all RuleRunners
func (o *OrchestratorV2) Start() error {
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
			// Skip hidden directories and common dependency folders
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") && path != projectRoot {
				return filepath.SkipDir
			}
			if base == "node_modules" || base == "vendor" || base == "gen" {
				return filepath.SkipDir
			}

			if err := o.Watcher.Add(path); err != nil {
				log.Printf("Error watching %s: %v", path, err)
			} else if o.Verbose {
				log.Printf("[devloop] Watching directory: %s", path)
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
func (o *OrchestratorV2) watchFiles() {
	for {
		select {
		case event, ok := <-o.Watcher.Events:
			if !ok {
				return
			}

			if o.Verbose {
				o.logDevloop("File event: %s on %s", event.Op, event.Name)
			}

			// Check which rules match this file
			o.runnersMutex.RLock()
			for _, runner := range o.ruleRunners {
				rule := runner.GetRule()
				if matcher := rule.Matches(event.Name); matcher != nil {
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
					if o.shouldTriggerByDefault(&rule) {
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
			log.Printf("[devloop] Watcher error: %v", err)

		case <-o.done:
			return
		}
	}
}

// shouldTriggerByDefault determines if a rule should trigger when no patterns match
func (o *OrchestratorV2) shouldTriggerByDefault(rule *gateway.Rule) bool {
	// Check rule-specific default first
	if rule.DefaultAction != "" {
		return rule.DefaultAction == "include"
	}
	// Fall back to global default
	return o.Config.Settings.DefaultWatchAction == "include"
}

// getDebounceDelayForRule returns the effective debounce delay for a rule
func (o *OrchestratorV2) getDebounceDelayForRule(rule gateway.Rule) time.Duration {
	// Rule-specific delay takes precedence
	if rule.DebounceDelay != nil {
		return *rule.DebounceDelay
	}
	// Fall back to global default
	if o.Config.Settings.DefaultDebounceDelay != nil {
		return *o.Config.Settings.DefaultDebounceDelay
	}
	// Final fallback to hardcoded default
	return 500 * time.Millisecond
}

// isVerboseForRule returns whether verbose logging is enabled for a rule
func (o *OrchestratorV2) isVerboseForRule(rule gateway.Rule) bool {
	// Rule-specific setting takes precedence
	if rule.Verbose != nil {
		return *rule.Verbose
	}
	// Fall back to global setting
	return o.Config.Settings.Verbose
}

// safeDone closes the done channel safely using sync.Once
func (o *OrchestratorV2) safeDone() {
	o.doneOnce.Do(func() {
		close(o.done)
	})
}

// Stop gracefully shuts down the orchestrator
func (o *OrchestratorV2) Stop() error {
	log.Println("[devloop] Stopping orchestrator...")
	o.safeDone()

	// Stop all RuleRunners
	var wg sync.WaitGroup
	o.runnersMutex.RLock()
	for name, runner := range o.ruleRunners {
		wg.Add(1)
		go func(n string, r RuleRunner) {
			defer wg.Done()
			if err := r.Stop(); err != nil {
				log.Printf("[devloop] Error stopping rule %q: %v", n, err)
			}
		}(name, runner)
	}
	o.runnersMutex.RUnlock()

	wg.Wait()
	log.Println("[devloop] All rules stopped")

	// Disconnect from gateway
	if o.gatewayStream != nil {
		o.disconnectFromGateway()
	}

	// Close log manager
	if err := o.LogManager.Close(); err != nil {
		log.Printf("Error closing log manager: %v", err)
	}

	return o.Watcher.Close()
}

// GetRuleStatus returns the status of a specific rule
func (o *OrchestratorV2) GetRuleStatus(ruleName string) (*gateway.RuleStatus, bool) {
	o.runnersMutex.RLock()
	defer o.runnersMutex.RUnlock()

	if runner, ok := o.ruleRunners[ruleName]; ok {
		return runner.GetStatus(), true
	}
	return nil, false
}

// ProjectRoot returns the project root directory
func (o *OrchestratorV2) ProjectRoot() string {
	return filepath.Dir(o.ConfigPath)
}

// connectToGateway establishes connection to the gateway service
func (o *OrchestratorV2) connectToGateway(gatewayAddr string) error {
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

	log.Printf("[devloop] Registered with gateway as project %q", o.projectID)

	// Start gateway communication handlers
	go o.handleGatewayStreamRecv()
	go o.handleGatewayStreamSend()

	return nil
}

// disconnectFromGateway cleanly disconnects from the gateway
func (o *OrchestratorV2) disconnectFromGateway() {
	unregisterMsg := &pb.DevloopMessage{
		Content: &pb.DevloopMessage_UnregisterRequest{
			UnregisterRequest: &pb.UnregisterRequest{
				ProjectId: o.projectID,
			},
		},
	}

	if err := o.gatewayStream.Send(unregisterMsg); err != nil {
		log.Printf("[devloop] Error sending unregister message to gateway: %v", err)
	}

	o.gatewayStream.CloseSend()
}

// handleGatewayStreamRecv handles incoming messages from gateway
func (o *OrchestratorV2) handleGatewayStreamRecv() {
	log.Println("[devloop] Starting gateway stream receiver.")
	for {
		select {
		case <-o.done:
			log.Println("[devloop] Gateway stream receiver stopping.")
			return
		default:
			msg, err := o.gatewayStream.Recv()
			if err == io.EOF {
				log.Println("[devloop] Gateway closed stream (EOF). Shutting down.")
				o.safeDone()
				return
			}
			if err != nil {
				log.Printf("[devloop] Error receiving from gateway stream: %v. Shutting down.", err)
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
				log.Printf("[devloop] Received unexpected RegisterRequest from gateway.")
			case *pb.DevloopMessage_UnregisterRequest:
				log.Printf("[devloop] Received unexpected UnregisterRequest from gateway.")
			case *pb.DevloopMessage_LogLine:
				log.Printf("[devloop] Received unexpected LogLine from gateway.")
			case *pb.DevloopMessage_UpdateRuleStatusRequest:
				log.Printf("[devloop] Received unexpected UpdateRuleStatusRequest from gateway.")
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
							log.Printf("[devloop] Response channel for correlation ID %s is full, dropping response.", msg.GetCorrelationId())
						}
					} else {
						log.Printf("[devloop] Received response for unknown correlation ID %s: %T", msg.GetCorrelationId(), content)
					}
				}
			}
		}
	}
}

// handleGatewayStreamSend sends outgoing messages to gateway
func (o *OrchestratorV2) handleGatewayStreamSend() {
	for {
		select {
		case <-o.done:
			return
		case msg := <-o.gatewaySendChan:
			if err := o.gatewayStream.Send(msg); err != nil {
				log.Printf("[devloop] Error sending message to gateway: %v", err)
				o.safeDone()
				return
			}
		}
	}
}

// GetWatchedPaths returns a unique list of all paths being watched by any rule
func (o *OrchestratorV2) GetWatchedPaths() []string {
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
func (o *OrchestratorV2) ReadFileContent(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// StreamLogs streams the logs for a given rule to the provided gRPC stream
func (o *OrchestratorV2) StreamLogs(ruleName string, filter string, stream pb.GatewayClientService_StreamLogsClientServer) error {
	return o.LogManager.StreamLogs(ruleName, filter, stream)
}

// TriggerRule manually triggers the execution of a specific rule
func (o *OrchestratorV2) TriggerRule(ruleName string) error {
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
func (o *OrchestratorV2) SetGlobalDebounceDelay(duration time.Duration) {
	o.runnersMutex.Lock()
	defer o.runnersMutex.Unlock()

	// Update the global setting
	o.Config.Settings.DefaultDebounceDelay = &duration

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
func (o *OrchestratorV2) SetRuleDebounceDelay(ruleName string, duration time.Duration) error {
	o.runnersMutex.Lock()
	defer o.runnersMutex.Unlock()

	// Find the rule and update its debounce delay
	for i := range o.Config.Rules {
		if o.Config.Rules[i].Name == ruleName {
			o.Config.Rules[i].DebounceDelay = &duration

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
func (o *OrchestratorV2) SetVerbose(verbose bool) {
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
func (o *OrchestratorV2) SetRuleVerbose(ruleName string, verbose bool) error {
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
func (o *OrchestratorV2) logDevloop(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)

	if o.Config.Settings.PrefixLogs && o.Config.Settings.PrefixMaxLength > 0 {
		// Format with left-aligned "devloop" prefix to match rule output format
		prefix := "devloop"
		totalPadding := o.Config.Settings.PrefixMaxLength - len(prefix)
		leftAlignedPrefix := prefix + strings.Repeat(" ", totalPadding)
		prefixStr := "[" + leftAlignedPrefix + "] "

		// Add color if enabled
		if o.ColorManager != nil && o.ColorManager.IsEnabled() {
			// Create a fake rule for devloop messages to get consistent coloring
			devloopRule := &gateway.Rule{Name: "devloop"}
			coloredPrefix := o.ColorManager.FormatPrefix(prefixStr, devloopRule)
			fmt.Printf("%s%s\n", coloredPrefix, message)
		} else {
			fmt.Printf("%s%s\n", prefixStr, message)
		}
	} else {
		// Standard log format but with devloop color if available
		if o.ColorManager != nil && o.ColorManager.IsEnabled() {
			devloopRule := &gateway.Rule{Name: "devloop"}
			coloredDevloop := o.ColorManager.FormatPrefix("[devloop]", devloopRule)
			log.Printf("%s %s", coloredDevloop, message)
		} else {
			log.Printf("[devloop] %s", message)
		}
	}
}
