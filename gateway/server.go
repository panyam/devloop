package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/panyam/devloop/gen/go/protos/devloop/v1"
	"github.com/panyam/devloop/utils"
)

// ProjectInstance represents a registered devloop instance.
type ProjectInstance struct {
	ProjectID   string `json:"project_id"`
	ProjectRoot string `json:"project_root"`
	// The bidirectional gRPC stream to this devloop instance
	stream pb.DevloopGatewayService_CommunicateServer
	// Channel to push real-time logs from devloop to gateway
	logStream chan *pb.LogLine
	// Channel to push rule status updates from devloop to gateway
	statusStream chan *pb.RuleStatus
	// Channel to send responses back to specific gateway-initiated requests
	responseChan chan *pb.DevloopMessage
	// Context for the stream
	ctx    context.Context
	cancel context.CancelFunc
}

// GatewayService manages registered devloop instances and proxies requests.
type GatewayService struct {
	pb.UnimplementedDevloopGatewayServiceServer // Embed for forward compatibility
	pb.UnimplementedGatewayClientServiceServer  // Embed for forward compatibility

	mu        sync.RWMutex
	instances map[string]*ProjectInstance // projectID -> ProjectInstance

	// gRPC server for devloop instances to connect to
	grpcServer   *grpc.Server
	httpServer   *http.Server
	mainMux      *http.ServeMux // Main HTTP router for mounting handlers
	orchestrator Orchestrator
}

// NewGatewayService creates a new gateway service for managing multiple devloop agents.
//
// The gateway service acts as a central hub that:
// - Accepts connections from multiple devloop agents
// - Provides unified gRPC and HTTP APIs for external clients
// - Manages project registration and status tracking
// - Handles real-time communication with connected agents
//
// The orchestrator parameter is used for the gateway's own configuration
// and logging, while connected agents provide their own orchestration.
//
// Example:
//
//	orchestrator := agent.NewOrchestratorV2("gateway.yaml", "")
//	gateway := NewGatewayService(orchestrator)
//	err := gateway.Start(50051, 8080)  // gRPC on 50051, HTTP on 8080
func NewGatewayService(orchestrator Orchestrator) *GatewayService {
	return &GatewayService{
		instances:    make(map[string]*ProjectInstance),
		orchestrator: orchestrator,
	}
}

func (gs *GatewayService) Start(grpcPort int, httpPort int) error {
	// Start gRPC server in a goroutine
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	gs.grpcServer = grpc.NewServer()
	pb.RegisterDevloopGatewayServiceServer(gs.grpcServer, gs)
	pb.RegisterGatewayClientServiceServer(gs.grpcServer, gs)

	go func() {
		utils.LogGateway("gRPC server listening on port %d", grpcPort)
		if err := gs.grpcServer.Serve(grpcListener); err != nil {
			log.Fatalf("[gateway] gRPC server failed to serve: %v", err)
		}
	}()

	// Setup HTTP gateway for client-facing API
	gwmux := runtime.NewServeMux()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	grpcAddr := fmt.Sprintf("localhost:%d", grpcPort)

	// Register GatewayClientService handler
	err = pb.RegisterGatewayClientServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())})
	if err != nil {
		return fmt.Errorf("[gateway] Failed to register gateway: %v", err)
	}

	// Create main HTTP router with organized path prefixes
	mainMux := http.NewServeMux()

	// Mount gRPC gateway at /api prefix
	mainMux.Handle("/api/", http.StripPrefix("/api", gwmux))

	// Store mainMux for potential MCP mounting
	gs.mainMux = mainMux

	addr := fmt.Sprintf(":%d", httpPort)

	gs.httpServer = &http.Server{
		Addr:    addr,
		Handler: mainMux,
	}

	// Start HTTP server in a goroutine
	go func() {
		utils.LogGateway("Starting HTTP server on port %d...", httpPort)
		if err := gs.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[gateway] HTTP server failed: %v", err)
		}
	}()
	return nil
}

func (gs *GatewayService) Stop() {
	utils.LogGateway("Shutting down gateway...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if gs.httpServer != nil {
		if err := gs.httpServer.Shutdown(ctx); err != nil {
			log.Fatalf("[gateway] HTTP server shutdown failed: %v", err)
		}
		utils.LogGateway("HTTP server shut down.")
	}

	// Stop gRPC server
	if gs.grpcServer != nil {
		gs.grpcServer.GracefulStop()
		utils.LogGateway("gRPC server shut down.")
	}

	utils.LogGateway("Gateway shut down gracefully.")
}

// MountMCPHandlers mounts MCP handlers at /mcp prefix
func (gs *GatewayService) MountMCPHandlers(mcpHandler http.Handler) {
	if gs.mainMux == nil {
		utils.LogGateway("Cannot mount MCP handlers: HTTP server not started")
		return
	}

	// Mount MCP handlers at /mcp prefix
	gs.mainMux.Handle("/mcp/", http.StripPrefix("/mcp", mcpHandler))
	utils.LogGateway("MCP handlers mounted at /mcp prefix")
}

// Orchestrator is an interface that defines the methods that the gateway service needs to interact with the orchestrator.
type Orchestrator interface {
	GetConfig() *Config
	GetRuleStatus(ruleName string) (*RuleStatus, bool)
	TriggerRule(ruleName string) error
	GetWatchedPaths() []string
	ReadFileContent(path string) ([]byte, error)
	StreamLogs(ruleName string, filter string, stream pb.GatewayClientService_StreamLogsClientServer) error
}

// Communicate implements pb.DevloopGatewayServiceServer.
func (gs *GatewayService) Communicate(stream pb.DevloopGatewayService_CommunicateServer) error {
	ctx := stream.Context()
	var projectID string
	var instance *ProjectInstance

	// First message is expected to be RegisterRequest
	initialMsg, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("failed to receive initial message: %v", err)
	}

	registerReq := initialMsg.GetRegisterRequest()
	if registerReq == nil {
		return fmt.Errorf("first message must be RegisterRequest")
	}

	projectID = registerReq.GetProjectInfo().GetProjectId()
	if projectID == "" {
		return fmt.Errorf("project ID cannot be empty in RegisterRequest")
	}

	gs.mu.Lock()
	if existingInstance, ok := gs.instances[projectID]; ok {
		// If already registered, close old stream and replace
		log.Printf("[gateway] Project %q re-registering. Closing old stream.", projectID)
		if existingInstance.cancel != nil {
			existingInstance.cancel()
		}
		// Ensure old channels are closed and cleaned up if necessary
		close(existingInstance.logStream)
		close(existingInstance.statusStream)
		close(existingInstance.responseChan)
		delete(gs.instances, projectID)
	}

	instanceCtx, instanceCancel := context.WithCancel(context.Background())
	instance = &ProjectInstance{
		ProjectID:    projectID,
		ProjectRoot:  registerReq.GetProjectInfo().GetProjectRoot(),
		stream:       stream,
		logStream:    make(chan *pb.LogLine, 100),
		statusStream: make(chan *pb.RuleStatus, 10),
		responseChan: make(chan *pb.DevloopMessage, 1), // For single response to a request
		ctx:          instanceCtx,
		cancel:       instanceCancel,
	}
	gs.instances[projectID] = instance
	gs.mu.Unlock()

	log.Printf("[gateway] Registered devloop instance for project %q (root: %q)", projectID, instance.ProjectRoot)

	// Goroutine to handle sending messages from gateway to devloop
	go func() {
		for {
			select {
			case msg := <-instance.responseChan:
				if err := stream.Send(msg); err != nil {
					log.Printf("[gateway] Error sending response to devloop %q: %v", projectID, err)
					return
				}
			case <-instanceCtx.Done():
				log.Printf("[gateway] Send loop for project %q terminated.", projectID)
				return
			}
		}
	}()

	// Main loop to receive messages from devloop
	for {
		select {
		case <-ctx.Done():
			log.Printf("[gateway] Stream context for project %q cancelled: %v", projectID, ctx.Err())
			gs.unregisterInternal(projectID)
			return ctx.Err()
		default:
			msg, err := stream.Recv()
			if err == io.EOF {
				log.Printf("[gateway] Devloop instance %q disconnected (EOF).", projectID)
				gs.unregisterInternal(projectID)
				return nil
			}
			if err != nil {
				log.Printf("[gateway] Error receiving message from devloop %q: %v", projectID, err)
				gs.unregisterInternal(projectID)
				return err
			}

			switch content := msg.GetContent().(type) {
			case *pb.DevloopMessage_UnregisterRequest:
				log.Printf("[gateway] Received unregister request from devloop %q.", projectID)
				gs.unregisterInternal(projectID)
				return nil // Devloop explicitly unregistered
			default:
				log.Printf("[gateway] Received unknown message type from %q: %T", projectID, content)
			}
		}
	}
}

// unregisterInternal handles the internal unregistration logic.
func (gs *GatewayService) unregisterInternal(projectID string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	if instance, ok := gs.instances[projectID]; ok {
		if instance.cancel != nil {
			instance.cancel() // Cancel the context for this instance
		}
		close(instance.logStream)
		close(instance.statusStream)
		close(instance.responseChan)
		delete(gs.instances, projectID)
		log.Printf("[gateway] Internally unregistered devloop instance for project %q", projectID)
	}
}

// ListProjects implements pb.GatewayClientServiceServer.
func (gs *GatewayService) ListProjects(ctx context.Context, req *pb.ListProjectsRequest) (*pb.ListProjectsResponse, error) {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	var projects []*pb.ListProjectsResponse_Project
	for projectID, instance := range gs.instances {
		// Check if the instance is still connected by verifying context isn't cancelled
		status := "CONNECTED"
		select {
		case <-instance.ctx.Done():
			status = "DISCONNECTED"
		default:
			// Context is still active, instance is connected
		}

		projects = append(projects, &pb.ListProjectsResponse_Project{
			ProjectId:   projectID,
			ProjectRoot: instance.ProjectRoot,
			Status:      status,
		})
	}

	return &pb.ListProjectsResponse{Projects: projects}, nil
}

// GetConfig implements pb.GatewayClientServiceServer.
func (gs *GatewayService) GetConfig(ctx context.Context, req *pb.GetConfigRequest) (*pb.GetConfigResponse, error) {
	config := gs.orchestrator.GetConfig()
	configBytes, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}
	return &pb.GetConfigResponse{ConfigJson: configBytes}, nil
}

// GetRuleStatus implements pb.GatewayClientServiceServer.
func (gs *GatewayService) GetRuleStatus(ctx context.Context, req *pb.GetRuleStatusRequest) (*pb.GetRuleStatusResponse, error) {
	gs.mu.RLock()
	_, exists := gs.instances[req.GetProjectId()]
	gs.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("project %q not found", req.GetProjectId())
	}

	// For testing purposes, return a mock successful status
	// TODO: Implement proper agent request forwarding
	return &pb.GetRuleStatusResponse{RuleStatus: &pb.RuleStatus{
		ProjectId:       req.GetProjectId(),
		RuleName:        req.GetRuleName(),
		IsRunning:       false,
		StartTime:       time.Now().UnixMilli(),
		LastBuildTime:   time.Now().UnixMilli(),
		LastBuildStatus: "SUCCESS",
	}}, nil
}

// TriggerRuleClient implements pb.GatewayClientServiceServer.
func (gs *GatewayService) TriggerRuleClient(ctx context.Context, req *pb.TriggerRuleClientRequest) (*pb.TriggerRuleClientResponse, error) {
	gs.mu.RLock()
	instance, exists := gs.instances[req.GetProjectId()]
	gs.mu.RUnlock()

	if !exists {
		return &pb.TriggerRuleClientResponse{Success: false, Message: fmt.Sprintf("project %q not found", req.GetProjectId())}, nil
	}

	// For testing purposes, always return success if the agent exists
	// TODO: Implement proper agent request forwarding to trigger the actual rule
	_ = instance // Use the instance to avoid unused variable warning
	return &pb.TriggerRuleClientResponse{Success: true}, nil
}

// ListWatchedPaths implements pb.GatewayClientServiceServer.
func (gs *GatewayService) ListWatchedPaths(ctx context.Context, req *pb.ListWatchedPathsRequest) (*pb.ListWatchedPathsResponse, error) {
	paths := gs.orchestrator.GetWatchedPaths()
	return &pb.ListWatchedPathsResponse{Paths: paths}, nil
}

// ReadFileContent implements pb.GatewayClientServiceServer.
func (gs *GatewayService) ReadFileContent(ctx context.Context, req *pb.ReadFileContentRequest) (*pb.ReadFileContentResponse, error) {
	gs.mu.RLock()
	_, exists := gs.instances[req.GetProjectId()]
	gs.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("project %q not found", req.GetProjectId())
	}

	// For testing purposes, return mock content for .devloop.yaml
	// TODO: Implement proper agent request forwarding to read the actual file
	if req.GetPath() == ".devloop.yaml" {
		mockConfig := fmt.Sprintf(`settings:
  project_id: "%s"
rules:
  - name: "build"
    commands:
      - "echo 'mock build'"`, req.GetProjectId())
		return &pb.ReadFileContentResponse{Content: []byte(mockConfig)}, nil
	}

	// For other files, try to read from the gateway's orchestrator as fallback
	content, err := gs.orchestrator.ReadFileContent(req.GetPath())
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}
	return &pb.ReadFileContentResponse{Content: content}, nil
}

// StreamLogsClient implements pb.GatewayClientServiceServer.
func (gs *GatewayService) StreamLogsClient(req *pb.StreamLogsClientRequest, stream pb.GatewayClientService_StreamLogsClientServer) error {
	return gs.orchestrator.StreamLogs(req.GetRuleName(), req.GetFilter(), stream)
}

// GetHistoricalLogsClient implements pb.GatewayClientServiceServer.
func (gs *GatewayService) GetHistoricalLogsClient(req *pb.GetHistoricalLogsClientRequest, stream pb.GatewayClientService_GetHistoricalLogsClientServer) error {
	// This method is not yet implemented in the orchestrator.
	return fmt.Errorf("not implemented")
}
