package agent

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"

	"github.com/panyam/devloop/gateway"
	pb "github.com/panyam/devloop/gen/go/protos/devloop/v1"
)

// createCrossPlatformCommand creates a command that works on both Unix and Windows
func createCrossPlatformCommand(cmdStr string) *exec.Cmd {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("cmd", "/c", cmdStr)
	default:
		// Unix-like systems (Linux, macOS, BSD, etc.)
		shell := "bash"
		// Check if bash exists, fallback to sh for better POSIX compatibility
		if _, err := exec.LookPath("bash"); err != nil {
			shell = "sh"
		}
		return exec.Command(shell, "-c", cmdStr)
	}
}

// Orchestrator manages file watching and rule execution.
type Orchestrator struct {
	ConfigPath       string
	Config           *gateway.Config
	Verbose          bool
	Watcher          *fsnotify.Watcher
	LogManager       *LogManager                                // New field for log management
	ColorManager     *ColorManager                              // Color management for rule output
	gatewayClient    pb.DevloopGatewayServiceClient             // gRPC client to the gateway
	gatewayStream    pb.DevloopGatewayService_CommunicateClient // Bidirectional stream to gateway
	projectID        string                                     // Unique ID for this devloop instance/project
	done             chan bool
	debounceTimers   map[string]*time.Timer
	debouncedEvents  chan gateway.Rule
	debounceDuration time.Duration
	runningCommands  map[string][]*exec.Cmd         // Track running commands by rule name
	ruleStatuses     map[string]*gateway.RuleStatus // Track status of each rule
	mu               sync.RWMutex                   // Mutex to protect ruleStatuses
	// Channels for gateway communication
	gatewaySendChan chan *pb.DevloopMessage            // Channel to send messages to gateway
	responseChan    map[string]chan *pb.DevloopMessage // Correlation ID -> response channel for gateway-initiated requests
	nextRequestID   int64
}

// executeCommands runs the commands associated with a rule.
func (o *Orchestrator) executeCommands(rule gateway.Rule) {
	o.mu.Lock()
	status, ok := o.ruleStatuses[rule.Name]
	if !ok {
		status = &gateway.RuleStatus{}
		o.ruleStatuses[rule.Name] = status
	}
	status.IsRunning = true
	status.StartTime = time.Now()
	status.LastBuildStatus = "RUNNING"
	o.mu.Unlock()

	if o.isVerboseForRule(rule) {
		log.Printf("[devloop] Executing commands for rule %q", rule.Name)
	}

	// Terminate any previously running commands for this rule
	if cmds, ok := o.runningCommands[rule.Name]; ok {
		for _, cmd := range cmds {
			if cmd != nil && cmd.Process != nil {
				pid := cmd.Process.Pid
				// Check if process still exists
				if err := syscall.Kill(pid, 0); err == nil {
					// Process exists, terminate it
					if o.isVerboseForRule(rule) {
						log.Printf("[devloop] Terminating previous process group %d for rule %q", pid, rule.Name)
					}
					// Send SIGTERM to the process group
					if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
						// Only log if it's not "no such process" error
						if !strings.Contains(err.Error(), "no such process") {
							log.Printf("Error sending SIGTERM to process group %d for rule %q: %v", pid, rule.Name, err)
						}
					}
				}
				// Wait for the process to exit (even if already dead)
				go func(c *exec.Cmd, pid int, r gateway.Rule) {
					_ = c.Wait() // This will return immediately if process is already dead
					if o.isVerboseForRule(r) {
						log.Printf("[devloop] Previous process group %d for rule %q terminated.", pid, r.Name)
					}
				}(cmd, pid, rule)
			}
		}
	}
	o.runningCommands[rule.Name] = []*exec.Cmd{} // Clear previous commands

	var currentCmds []*exec.Cmd
	logWriter, err := o.LogManager.GetWriter(rule.Name)
	if err != nil {
		log.Printf("[devloop] Error getting log writer for rule %q: %v", rule.Name, err)
		return
	}

	// Execute commands sequentially
	var lastCmd *exec.Cmd
	for i, cmdStr := range rule.Commands {
		log.Printf("[devloop]   Running command: %s", cmdStr)
		cmd := createCrossPlatformCommand(cmdStr)

		writers := []io.Writer{os.Stdout, logWriter}

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

			// Use ColoredPrefixWriter for enhanced output with color support
			prefixStr := "[" + prefix + "] "
			coloredWriter := NewColoredPrefixWriter(writers, prefixStr, o.ColorManager, &rule)
			cmd.Stdout = coloredWriter
			cmd.Stderr = coloredWriter
		} else {
			// For non-prefixed output, still use ColoredPrefixWriter but with empty prefix
			// This ensures consistent color handling even without prefixes
			if o.ColorManager.IsEnabled() {
				coloredWriter := NewColoredPrefixWriter(writers, "", o.ColorManager, &rule)
				cmd.Stdout = coloredWriter
				cmd.Stderr = coloredWriter
			} else {
				multiWriter := io.MultiWriter(writers...)
				cmd.Stdout = multiWriter
				cmd.Stderr = multiWriter
			}
		}

		// Set platform-specific process attributes
		setSysProcAttr(cmd)

		// Set working directory - default to config file directory if not specified
		workDir := rule.WorkDir
		if workDir == "" {
			workDir = filepath.Dir(o.ConfigPath)
		}
		cmd.Dir = workDir

		// Set environment variables
		cmd.Env = os.Environ() // Inherit parent environment
		for key, value := range rule.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}

		err := cmd.Start()
		if err != nil {
			log.Printf("[devloop] Command %q failed to start for rule %q: %v", cmdStr, rule.Name, err)
			// Stop executing further commands in this rule
			break
		}
		currentCmds = append(currentCmds, cmd)

		// For the last command or if it's a long-running command (like a server),
		// we don't wait. Otherwise, wait for the command to complete before proceeding.
		if i < len(rule.Commands)-1 {
			// Wait for non-final commands to complete
			waitErr := cmd.Wait()
			if waitErr != nil {
				log.Printf("[devloop] Command failed for rule %q: %v", rule.Name, waitErr)
				// Stop executing further commands if one fails
				break
			}
		} else {
			// This is the last command
			lastCmd = cmd
		}
	}
	o.runningCommands[rule.Name] = currentCmds

	// Signal that the rule has finished when the last command is done
	if lastCmd != nil {
		go func() {
			waitErr := lastCmd.Wait() // Wait for the last command to finish

			o.mu.Lock()
			defer o.mu.Unlock()
			status := o.ruleStatuses[rule.Name]
			if status != nil {
				status.IsRunning = false
				status.LastBuildTime = time.Now()
				if waitErr == nil {
					status.LastBuildStatus = "SUCCESS"
				} else {
					log.Printf("[devloop] Command for rule %q failed: %v", rule.Name, waitErr)
					status.LastBuildStatus = "FAILED"
				}

				// Send rule status update to gateway
				if o.gatewayStream != nil {
					statusMsg := &pb.DevloopMessage{
						Content: &pb.DevloopMessage_UpdateRuleStatusRequest{
							UpdateRuleStatusRequest: &pb.UpdateRuleStatusRequest{
								RuleStatus: &pb.RuleStatus{
									ProjectId:       o.projectID,
									RuleName:        status.RuleName,
									IsRunning:       status.IsRunning,
									StartTime:       status.StartTime.UnixMilli(),
									LastBuildTime:   status.LastBuildTime.UnixMilli(),
									LastBuildStatus: status.LastBuildStatus,
								},
							},
						},
					}
					select {
					case o.gatewaySendChan <- statusMsg:
					default:
						log.Printf("[devloop] Failed to send rule status update for %q: channel full", rule.Name)
					}
				}
			}

			o.LogManager.SignalFinished(rule.Name)
			log.Printf("[devloop] Rule %q commands finished.", rule.Name)
		}()
	}
}

// debounce manages the debounce timer for a given rule.
func (o *Orchestrator) debounce(rule gateway.Rule) {
	if timer, ok := o.debounceTimers[rule.Name]; ok {
		timer.Stop() // Reset the existing timer
	}

	// Get the effective debounce delay for this rule
	delay := o.getDebounceDelayForRule(rule)

	o.debounceTimers[rule.Name] = time.AfterFunc(delay, func() {
		o.debouncedEvents <- rule // Send rule to debouncedEvents channel after duration
	})
}

// LoadConfig reads and unmarshals the .devloop.yaml configuration file,
// resolving all relative watch paths to be absolute from the config file's location.
func LoadConfig(configPath string) (*gateway.Config, error) {
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for config file: %w", err)
	}

	data, err := os.ReadFile(absConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %q", absConfigPath)
		}
		return nil, fmt.Errorf("failed to read config file %q: %w", absConfigPath, err)
	}

	var config gateway.Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", absConfigPath, err)
	}

	// Set defaults and resolve relative watch paths to be absolute.
	configDir := filepath.Dir(absConfigPath)

	// First pass: parse YAML to check which fields were explicitly set
	var rawConfig map[string]interface{}
	yaml.Unmarshal(data, &rawConfig)

	for i := range config.Rules {
		rule := &config.Rules[i]

		// Set default for RunOnInit to true if not explicitly set
		if rules, ok := rawConfig["rules"].([]interface{}); ok && i < len(rules) {
			if ruleMap, ok := rules[i].(map[string]interface{}); ok {
				if _, hasRunOnInit := ruleMap["run_on_init"]; !hasRunOnInit {
					rule.RunOnInit = true // Default to true if not specified
				}
			}
		} else {
			// If we can't determine, default to true
			rule.RunOnInit = true
		}

		for _, matcher := range rule.Watch {
			for j, pattern := range matcher.Patterns {
				if !filepath.IsAbs(pattern) {
					absPattern := filepath.Join(configDir, pattern)
					matcher.Patterns[j] = absPattern
				}
			}
		}
	}

	return &config, nil
}

func (o *Orchestrator) ProjectRoot() string {
	return filepath.Dir(o.ConfigPath)
}

// NewOrchestrator creates a new Orchestrator instance.
func NewOrchestrator(configPath string, gatewayAddr string) (*Orchestrator, error) {
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

	// The project root is the directory containing the config file.
	projectRoot := filepath.Dir(absConfigPath)

	// Generate a unique project ID based on the project root.
	hasher := sha1.New()
	hasher.Write([]byte(projectRoot))
	projectID := hex.EncodeToString(hasher.Sum(nil))

	orchestrator := &Orchestrator{
		ConfigPath:       absConfigPath,
		Config:           config,
		Watcher:          watcher,
		projectID:        projectID,
		done:             make(chan bool),
		debounceTimers:   make(map[string]*time.Timer),
		debouncedEvents:  make(chan gateway.Rule),
		debounceDuration: 500 * time.Millisecond,
		runningCommands:  make(map[string][]*exec.Cmd),
		ruleStatuses:     make(map[string]*gateway.RuleStatus),
		gatewaySendChan:  make(chan *pb.DevloopMessage, 100),
		responseChan:     make(map[string]chan *pb.DevloopMessage),
		nextRequestID:    0,
	}

	// Initialize LogManager after orchestrator is created
	logManager, err := NewLogManager("./logs")
	if err != nil {
		return nil, fmt.Errorf("failed to create log manager: %w", err)
	}
	orchestrator.LogManager = logManager

	// Initialize ColorManager
	orchestrator.ColorManager = NewColorManager(&config.Settings)

	// Connect to gateway if address is provided
	if gatewayAddr != "" {
		conn, err := grpc.NewClient(gatewayAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, fmt.Errorf("failed to connect to gateway %q: %w", gatewayAddr, err)
		}
		orchestrator.gatewayClient = pb.NewDevloopGatewayServiceClient(conn)

		// Establish the bidirectional communication stream
		stream, err := orchestrator.gatewayClient.Communicate(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to open gateway communication stream: %w", err)
		}
		orchestrator.gatewayStream = stream

		// Send initial registration message
		registerMsg := &pb.DevloopMessage{
			Content: &pb.DevloopMessage_RegisterRequest{
				RegisterRequest: &pb.RegisterRequest{
					ProjectInfo: &pb.ProjectInfo{
						ProjectId:   orchestrator.projectID,
						ProjectRoot: projectRoot,
					},
				},
			},
		}
		if err := orchestrator.gatewayStream.Send(registerMsg); err != nil {
			return nil, fmt.Errorf("failed to send registration to gateway: %w", err)
		}
		log.Printf("[devloop] Sent registration to gateway as project %q", orchestrator.projectID)

		// Start goroutine to receive messages from gateway
		go orchestrator.handleGatewayStreamRecv()

		// Start goroutine to send messages to gateway
		go orchestrator.handleGatewayStreamSend()
	}

	return orchestrator, nil
}

// handleGetConfigRequest handles a GetConfig request from the gateway.
func (o *Orchestrator) handleGetConfigRequest(correlationID string, req *pb.GetConfigRequest) {
	log.Printf("[devloop] Received GetConfig request for project %q (correlation ID: %s)", req.GetProjectId(), correlationID)

	configJSON, err := yaml.Marshal(o.Config)
	if err != nil {
		log.Printf("[devloop] Error marshaling config for project %q: %v", req.GetProjectId(), err)
		// Send an error response back to gateway
		respMsg := &pb.DevloopMessage{
			CorrelationId: correlationID,
			Content: &pb.DevloopMessage_GetConfigResponse{
				GetConfigResponse: &pb.GetConfigResponse{ConfigJson: []byte(fmt.Sprintf("Error: %v", err))},
			},
		}
		select {
		case o.gatewaySendChan <- respMsg:
		default:
			log.Printf("[devloop] Failed to send GetConfigResponse for %s: channel full", correlationID)
		}
		return
	}

	respMsg := &pb.DevloopMessage{
		CorrelationId: correlationID,
		Content: &pb.DevloopMessage_GetConfigResponse{
			GetConfigResponse: &pb.GetConfigResponse{ConfigJson: configJSON},
		},
	}

	select {
	case o.gatewaySendChan <- respMsg:
		log.Printf("[devloop] Sent GetConfigResponse for project %q (correlation ID: %s)", req.GetProjectId(), correlationID)
	default:
		log.Printf("[devloop] Failed to send GetConfigResponse for %s: channel full", correlationID)
	}
}

// GetConfig returns the orchestrator's configuration.
func (o *Orchestrator) GetConfig() *gateway.Config {
	return o.Config
}

// GetRuleStatus returns the current status of a specific rule.
func (o *Orchestrator) GetRuleStatus(ruleName string) (*gateway.RuleStatus, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	status, ok := o.ruleStatuses[ruleName]
	return status, ok
}

// handleGetRuleStatusRequest handles a GetRuleStatus request from the gateway.
func (o *Orchestrator) handleGetRuleStatusRequest(correlationID string, req *pb.GetRuleStatusRequest) {
	log.Printf("[devloop] Received GetRuleStatus request for project %q, rule %q (correlation ID: %s)", req.GetProjectId(), req.GetRuleName(), correlationID)

	status, ok := o.GetRuleStatus(req.GetRuleName())
	if !ok {
		log.Printf("[devloop] Rule %q not found for status request from gateway.", req.GetRuleName())
		respMsg := &pb.DevloopMessage{
			CorrelationId: correlationID,
			Content: &pb.DevloopMessage_GetRuleStatusResponse{
				GetRuleStatusResponse: &pb.GetRuleStatusResponse{RuleStatus: &pb.RuleStatus{ProjectId: req.GetProjectId(), RuleName: req.GetRuleName(), LastBuildStatus: "NOT_FOUND"}},
			},
		}
		select {
		case o.gatewaySendChan <- respMsg:
		default:
			log.Printf("[devloop] Failed to send GetRuleStatusResponse for %s: channel full", correlationID)
		}
		return
	}

	respMsg := &pb.DevloopMessage{
		CorrelationId: correlationID,
		Content: &pb.DevloopMessage_GetRuleStatusResponse{
			GetRuleStatusResponse: &pb.GetRuleStatusResponse{
				RuleStatus: &pb.RuleStatus{
					ProjectId:       o.projectID,
					RuleName:        status.RuleName,
					IsRunning:       status.IsRunning,
					StartTime:       status.StartTime.UnixMilli(),
					LastBuildTime:   status.LastBuildTime.UnixMilli(),
					LastBuildStatus: status.LastBuildStatus,
				},
			},
		},
	}

	select {
	case o.gatewaySendChan <- respMsg:
		log.Printf("[devloop] Sent GetRuleStatusResponse for project %q, rule %q (correlation ID: %s)", req.GetProjectId(), req.GetRuleName(), correlationID)
	default:
		log.Printf("[devloop] Failed to send GetRuleStatusResponse for %s: channel full", correlationID)
	}
}

// TriggerRule manually triggers the execution of a specific rule.
func (o *Orchestrator) TriggerRule(ruleName string) error {
	o.mu.RLock()
	defer o.mu.RUnlock()

	for _, rule := range o.Config.Rules {
		if rule.Name == ruleName {
			// Send the rule to the debouncedEvents channel to trigger execution
			o.debouncedEvents <- rule
			return nil
		}
	}
	return fmt.Errorf("rule %q not found", ruleName)
}

// handleTriggerRuleRequest handles a TriggerRule request from the gateway.
func (o *Orchestrator) handleTriggerRuleRequest(correlationID string, req *pb.TriggerRuleRequest) {
	log.Printf("[devloop] Received TriggerRule request for project %q, rule %q (correlation ID: %s)", req.GetProjectId(), req.GetRuleName(), correlationID)

	err := o.TriggerRule(req.GetRuleName())
	respMsg := &pb.DevloopMessage{
		CorrelationId: correlationID,
		Content: &pb.DevloopMessage_TriggerRuleResponse{
			TriggerRuleResponse: &pb.TriggerRuleResponse{Success: err == nil},
		},
	}
	if err != nil {
		respMsg.GetTriggerRuleResponse().Message = err.Error()
	}

	select {
	case o.gatewaySendChan <- respMsg:
		log.Printf("[devloop] Sent TriggerRuleResponse for project %q, rule %q (correlation ID: %s)", req.GetProjectId(), req.GetRuleName(), correlationID)
	default:
		log.Printf("[devloop] Failed to send TriggerRuleResponse for %s: channel full", correlationID)
	}
}

// handleListWatchedPathsRequest handles a ListWatchedPaths request from the gateway.
func (o *Orchestrator) handleListWatchedPathsRequest(correlationID string, req *pb.ListWatchedPathsRequest) {
	log.Printf("[devloop] Received ListWatchedPaths request for project %q (correlation ID: %s)", req.GetProjectId(), correlationID)

	paths := o.GetWatchedPaths()
	respMsg := &pb.DevloopMessage{
		CorrelationId: correlationID,
		Content: &pb.DevloopMessage_ListWatchedPathsResponse{
			ListWatchedPathsResponse: &pb.ListWatchedPathsResponse{Paths: paths},
		},
	}

	select {
	case o.gatewaySendChan <- respMsg:
		log.Printf("[devloop] Sent ListWatchedPathsResponse for project %q (correlation ID: %s)", req.GetProjectId(), correlationID)
	default:
		log.Printf("[devloop] Failed to send ListWatchedPathsResponse for %s: channel full", correlationID)
	}
}

// handleReadFileContentRequest handles a ReadFileContent request from the gateway.
func (o *Orchestrator) handleReadFileContentRequest(correlationID string, req *pb.ReadFileContentRequest) {
	log.Printf("[devloop] Received ReadFileContent request for project %q, path %q (correlation ID: %s)", req.GetProjectId(), req.GetPath(), correlationID)

	content, err := os.ReadFile(req.GetPath())
	respMsg := &pb.DevloopMessage{
		CorrelationId: correlationID,
		Content: &pb.DevloopMessage_ReadFileContentResponse{
			ReadFileContentResponse: &pb.ReadFileContentResponse{},
		},
	}

	if err != nil {
		log.Printf("[devloop] Error reading file %q for gateway request: %v", req.GetPath(), err)
		respMsg.GetReadFileContentResponse().Content = []byte(fmt.Sprintf("Error: %v", err))
	} else {
		respMsg.GetReadFileContentResponse().Content = content
	}

	select {
	case o.gatewaySendChan <- respMsg:
		log.Printf("[devloop] Sent ReadFileContentResponse for project %q, path %q (correlation ID: %s)", req.GetProjectId(), req.GetPath(), correlationID)
	default:
		log.Printf("[devloop] Failed to send ReadFileContentResponse for %s: channel full", correlationID)
	}
}

// handleGetHistoricalLogsRequest handles a GetHistoricalLogs request from the gateway.
func (o *Orchestrator) handleGetHistoricalLogsRequest(correlationID string, req *pb.GetHistoricalLogsRequest) {
	log.Printf("[devloop] Received GetHistoricalLogs request for project %q, rule %q (filter: %q, start: %d, end: %d) (correlation ID: %s)",
		req.GetProjectId(), req.GetRuleName(), req.GetFilter(), req.GetStartTime(), req.GetEndTime(), correlationID)

	logFilePath := filepath.Join(o.LogManager.logDir, fmt.Sprintf("%s.log", req.GetRuleName()))
	file, err := os.OpenFile(logFilePath, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Printf("[devloop] Error opening historical log file %q: %v", logFilePath, err)
		// Send an error response or just close the stream
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// TODO: Apply filter and time range here
		logLineMsg := &pb.DevloopMessage{
			CorrelationId: correlationID, // Use correlation ID to associate with the historical request
			Content: &pb.DevloopMessage_LogLine{
				LogLine: &pb.LogLine{
					ProjectId: o.projectID,
					RuleName:  req.GetRuleName(),
					Line:      line,
					Timestamp: time.Now().UnixMilli(), // Use actual log timestamp if available
				},
			},
		}
		select {
		case o.gatewaySendChan <- logLineMsg:
		default:
			log.Printf("[devloop] Failed to send historical log line for %s: channel full", correlationID)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[devloop] Error reading historical log file %q: %v", logFilePath, err)
	}

	// Signal end of historical logs by sending a special message or closing the stream
	// For now, we'll rely on the gateway detecting stream end or a timeout.
}

// ReadFileContent reads and returns the content of a specified file.
func (o *Orchestrator) ReadFileContent(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// GetWatchedPaths returns a unique list of all paths being watched by any rule.
func (o *Orchestrator) GetWatchedPaths() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()

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

// handleGatewayStreamRecv receives messages from the gateway.
func (o *Orchestrator) handleGatewayStreamRecv() {
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
				close(o.done) // Signal orchestrator to stop
				return
			}
			if err != nil {
				log.Printf("[devloop] Error receiving from gateway stream: %v. Shutting down.", err)
				close(o.done) // Signal orchestrator to stop
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
				// This should not happen after initial registration
				log.Printf("[devloop] Received unexpected RegisterRequest from gateway.")
			case *pb.DevloopMessage_UnregisterRequest:
				// This should not happen, unregister is devloop initiated
				log.Printf("[devloop] Received unexpected UnregisterRequest from gateway.")
			case *pb.DevloopMessage_LogLine:
				// This should not happen, logs are devloop initiated
				log.Printf("[devloop] Received unexpected LogLine from gateway.")
			case *pb.DevloopMessage_UpdateRuleStatusRequest:
				// This should not happen, status updates are devloop initiated
				log.Printf("[devloop] Received unexpected UpdateRuleStatusRequest from gateway.")
			default:
				// This is a response to a devloop-initiated request
				if msg.GetCorrelationId() != "" {
					o.mu.RLock()
					respChan, ok := o.responseChan[msg.GetCorrelationId()]
					o.mu.RUnlock()
					if ok {
						select {
						case respChan <- msg:
						default:
							log.Printf("[devloop] Response channel for correlation ID %s is full, dropping response.", msg.GetCorrelationId())
						}
					} else {
						log.Printf("[devloop] Received response for unknown correlation ID %s: %T", msg.GetCorrelationId(), content)
					}
				} else {
					log.Printf("[devloop] Received unknown message type with no correlation ID: %T", content)
				}
			}
		}
	}
}

// handleGatewayStreamSend sends messages to the gateway.
func (o *Orchestrator) handleGatewayStreamSend() {
	log.Println("[devloop] Starting gateway stream sender.")
	for {
		select {
		case <-o.done:
			log.Println("[devloop] Gateway stream sender stopping.")
			return
		case msg := <-o.gatewaySendChan:
			if err := o.gatewayStream.Send(msg); err != nil {
				log.Printf("[devloop] Error sending message to gateway: %v. Shutting down.", err)
				close(o.done) // Signal orchestrator to stop
				return
			}
		}
	}
}

// Start begins the file watching and command execution loop.
func (o *Orchestrator) Start() error {
	// Execute rules with RunOnInit: true before starting file watching
	for _, rule := range o.Config.Rules {
		if rule.RunOnInit {
			if o.isVerboseForRule(rule) {
				log.Printf("[devloop] Executing rule %q on initialization (run_on_init: true)", rule.Name)
			}
			o.executeCommands(rule)
		}
	}

	// Walk the project root directory and add all subdirectories to the watcher
	projectRoot := o.ProjectRoot()
	if o.Verbose {
		log.Printf("[devloop] Starting file watcher from project root: %s", projectRoot)
	}
	err := filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Basic ignore for hidden directories and common dependency folders
			if strings.HasPrefix(filepath.Base(path), ".") && path != projectRoot {
				return filepath.SkipDir
			}
			if filepath.Base(path) == "node_modules" || filepath.Base(path) == "vendor" || filepath.Base(path) == "gen" {
				return filepath.SkipDir
			}
			err = o.Watcher.Add(path)
			if err != nil {
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

	go func() {
		for {
			select {
			case event, ok := <-o.Watcher.Events:
				if !ok {
					return
				}
				if o.Verbose {
					log.Printf("[devloop] File event: %s on %s", event.Op, event.Name)
				}
				// Check all rules for a match
				for _, rule := range o.Config.Rules {
					matcher := rule.Matches(event.Name)
					if matcher != nil {
						if o.isVerboseForRule(rule) {
							log.Printf("[devloop] Rule %q matched for file %s", rule.Name, event.Name)
						}
						// Use a map to debounce rules by name, ensuring each rule is only debounced once per event burst
						o.debounce(rule)
					}
				}
			case err, ok := <-o.Watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			case <-o.done:
				return
			}
		}
	}()

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

	<-o.done
	return nil
}

// StreamLogs streams the logs for a given rule to the provided gRPC stream.
func (o *Orchestrator) StreamLogs(ruleName string, filter string, stream pb.GatewayClientService_StreamLogsClientServer) error {
	return o.LogManager.StreamLogs(ruleName, filter, stream)
}

// SetGlobalDebounceDelay sets the default debounce delay for all rules
func (o *Orchestrator) SetGlobalDebounceDelay(duration time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.debounceDuration = duration
}

// SetRuleDebounceDelay sets the debounce delay for a specific rule
func (o *Orchestrator) SetRuleDebounceDelay(ruleName string, duration time.Duration) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Find the rule and update its debounce delay
	for i := range o.Config.Rules {
		if o.Config.Rules[i].Name == ruleName {
			o.Config.Rules[i].DebounceDelay = &duration
			return nil
		}
	}
	return fmt.Errorf("rule %q not found", ruleName)
}

// SetVerbose sets the global verbose flag
func (o *Orchestrator) SetVerbose(verbose bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.Verbose = verbose
	o.Config.Settings.Verbose = verbose
}

// SetRuleVerbose sets the verbose flag for a specific rule
func (o *Orchestrator) SetRuleVerbose(ruleName string, verbose bool) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Find the rule and update its verbose flag
	for i := range o.Config.Rules {
		if o.Config.Rules[i].Name == ruleName {
			o.Config.Rules[i].Verbose = &verbose
			return nil
		}
	}
	return fmt.Errorf("rule %q not found", ruleName)
}

// getDebounceDelayForRule returns the effective debounce delay for a rule
func (o *Orchestrator) getDebounceDelayForRule(rule gateway.Rule) time.Duration {
	// Check rule-specific setting first
	if rule.DebounceDelay != nil {
		return *rule.DebounceDelay
	}

	// Check global setting
	if o.Config.Settings.DefaultDebounceDelay != nil {
		return *o.Config.Settings.DefaultDebounceDelay
	}

	// Fall back to orchestrator's default
	return o.debounceDuration
}

// isVerboseForRule returns whether verbose logging is enabled for a rule
func (o *Orchestrator) isVerboseForRule(rule gateway.Rule) bool {
	// Check rule-specific setting first
	if rule.Verbose != nil {
		return *rule.Verbose
	}

	// Fall back to global setting
	return o.Verbose || o.Config.Settings.Verbose
}

// Stop gracefully shuts down the orchestrator.
func (o *Orchestrator) Stop() error {
	log.Println("[devloop] Stopping orchestrator...")
	close(o.done)

	// If connected to gateway, send unregister message
	if o.gatewayStream != nil {
		unregisterMsg := &pb.DevloopMessage{
			Content: &pb.DevloopMessage_UnregisterRequest{
				UnregisterRequest: &pb.UnregisterRequest{ProjectId: o.projectID},
			},
		}
		if err := o.gatewayStream.Send(unregisterMsg); err != nil {
			log.Printf("[devloop] Error sending unregister message to gateway: %v", err)
		}
		o.gatewayStream.CloseSend() // Close the send side of the stream
	}

	// Terminate all running commands
	var wg sync.WaitGroup
	for ruleName, cmds := range o.runningCommands {
		for _, cmd := range cmds {
			if cmd != nil && cmd.Process != nil {
				wg.Add(1)
				go func(c *exec.Cmd, rName string) {
					defer wg.Done()
					pid := c.Process.Pid
					log.Printf("Terminating process group %d for rule %q during shutdown", pid, rName)

					// Check if process still exists before trying to kill it
					if err := syscall.Kill(pid, 0); err != nil {
						// Process already dead
						log.Printf("Process %d for rule %q already terminated", pid, rName)
						return
					}

					// First try SIGTERM to allow graceful shutdown
					if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
						if !strings.Contains(err.Error(), "no such process") {
							log.Printf("Error sending SIGTERM to process group %d: %v", pid, err)
						}
					}

					// Give process a moment to exit gracefully
					done := make(chan bool)
					go func() {
						c.Wait()
						done <- true
					}()

					select {
					case <-done:
						log.Printf("Process group %d for rule %q terminated gracefully", pid, rName)
					case <-time.After(2 * time.Second):
						// Force kill if still running
						log.Printf("Force killing process group %d for rule %q", pid, rName)
						if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
							log.Printf("Error sending SIGKILL to process group %d: %v", pid, err)
						}
						// Also try to kill the specific process
						if err := c.Process.Kill(); err != nil {
							log.Printf("Error killing process %d: %v", pid, err)
						}
						<-done // Wait for the goroutine to finish
					}

					// Double-check by trying to kill again (will fail if process is gone)
					if err := syscall.Kill(pid, 0); err == nil {
						log.Printf("WARNING: Process %d still exists after termination attempts", pid)
						// One final attempt with SIGKILL directly to the process
						syscall.Kill(pid, syscall.SIGKILL)
					}
				}(cmd, ruleName)
			}
		}
	}

	// Wait for all termination goroutines to complete
	wg.Wait()
	log.Println("[devloop] All processes terminated")

	// Final cleanup check for any orphaned processes
	o.cleanupOrphanedProcesses()

	// Close the log manager
	if err := o.LogManager.Close(); err != nil {
		log.Printf("Error closing log manager: %v", err)
	}

	return o.Watcher.Close()
}
