package main

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

	if verbose {
		log.Printf("[devloop] Executing commands for rule %q", rule.Name)
	}

	// Terminate any previously running commands for this rule
	if cmds, ok := o.runningCommands[rule.Name]; ok {
		for _, cmd := range cmds {
			if cmd != nil && cmd.Process != nil {
				if verbose {
					log.Printf("[devloop] Terminating previous process group %d for rule %q", cmd.Process.Pid, rule.Name)
				}
				// Send SIGTERM to the process group
				err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
				if err != nil {
					log.Printf("Error sending SIGTERM to process group %d for rule %q: %v", cmd.Process.Pid, rule.Name, err)
				}
				// Wait for the process to exit
				go func(c *exec.Cmd) {
					_ = c.Wait() // Wait for the process to actually exit
					if verbose {
						log.Printf("[devloop] Previous process group %d for rule %q terminated.", c.Process.Pid, rule.Name)
					}
				}(cmd)
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

	for _, cmdStr := range rule.Commands {
		log.Printf("[devloop]   Running command: %s", cmdStr)
		cmd := exec.Command("bash", "-c", cmdStr)

		var writers []io.Writer
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
			writers = []io.Writer{os.Stdout, logWriter}
			cmd.Stdout = &PrefixWriter{writers: writers, prefix: "[" + prefix + "] "}
			cmd.Stderr = &PrefixWriter{writers: writers, prefix: "[" + prefix + "] "}
		} else {
			writers = []io.Writer{os.Stdout, logWriter}
			cmd.Stdout = io.MultiWriter(writers...)
			cmd.Stderr = io.MultiWriter(writers...)
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
			log.Printf("[devloop] Command %q failed to start for rule %q: %v", cmdStr, rule.Name, err)
			continue
		}
		currentCmds = append(currentCmds, cmd)

		// For long-running commands, we don't wait here. They run in background.
		// For now, we'll assume all commands might be long-running and don't wait.
	}
	o.runningCommands[rule.Name] = currentCmds

	// Signal that the rule has finished when all commands are done
	go func() {
		allCommandsSuccessful := true
		for _, cmd := range currentCmds {
			if cmd != nil && cmd.Process != nil {
				waitErr := cmd.Wait() // Wait for each command to finish
				if waitErr != nil {
					log.Printf("[devloop] Command for rule %q failed: %v", rule.Name, waitErr)
					allCommandsSuccessful = false
				}
			}
		}

		o.mu.Lock()
		defer o.mu.Unlock()
		status := o.ruleStatuses[rule.Name]
		if status != nil {
			status.IsRunning = false
			status.LastBuildTime = time.Now()
			if allCommandsSuccessful {
				status.LastBuildStatus = "SUCCESS"
			} else {
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

// Orchestrator manages file watching and rule execution.
type Orchestrator struct {
	ConfigPath       string
	Config           *gateway.Config
	Watcher          *fsnotify.Watcher
	LogManager       *LogManager                                // New field for log management
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

// debounce manages the debounce timer for a given rule.
func (o *Orchestrator) debounce(rule gateway.Rule) {
	if timer, ok := o.debounceTimers[rule.Name]; ok {
		timer.Stop() // Reset the existing timer
	}
	o.debounceTimers[rule.Name] = time.AfterFunc(o.debounceDuration, func() {
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

	// Resolve relative watch paths to be absolute.
	configDir := filepath.Dir(absConfigPath)
	for _, rule := range config.Rules {
		for _, matcher := range rule.Watch {
			for i, pattern := range matcher.Patterns {
				if !filepath.IsAbs(pattern) {
					absPattern := filepath.Join(configDir, pattern)
					matcher.Patterns[i] = absPattern
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
	// Walk the project root directory and add all subdirectories to the watcher
	projectRoot := o.ProjectRoot()
	if verbose {
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
			} else if verbose {
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
				if verbose {
					log.Printf("[devloop] File event: %s on %s", event.Op, event.Name)
				}
				// Check all rules for a match
				for _, rule := range o.Config.Rules {
					matcher := rule.Matches(event.Name)
					if matcher != nil {
						if verbose {
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
	for ruleName, cmds := range o.runningCommands {
		for _, cmd := range cmds {
			if cmd != nil && cmd.Process != nil {
				log.Printf("Terminating process group %d for rule %q during shutdown", cmd.Process.Pid, ruleName)
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			}
		}
	}

	// Close the log manager
	if err := o.LogManager.Close(); err != nil {
		log.Printf("Error closing log manager: %v", err)
	}

	return o.Watcher.Close()
}
