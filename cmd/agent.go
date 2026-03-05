package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/panyam/devloop/agent"
	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"github.com/panyam/devloop/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

type AgentConfig struct {
	ConfigPath  string
	Verbose     bool
	HTTPPort    int
	GRPCPort    int
	GatewayAddr string
	AutoPorts   bool
}

type Agent struct {
	config       *AgentConfig
	orchestrator *agent.Orchestrator
	grpcServer   *grpc.Server
	httpServer   *http.Server
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewAgent creates a new server instance with the given configuration
func NewAgent(config *AgentConfig) (*Agent, error) {
	orchestrator, err := agent.NewOrchestrator(config.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize orchestrator: %w", err)
	}
	orchestrator.Verbose = config.Verbose

	// Initialize global logger for consistent formatting across all components
	utils.InitGlobalLogger(
		orchestrator.Config.Settings.PrefixLogs,
		int(orchestrator.Config.Settings.PrefixMaxLength),
		orchestrator.ColorManager,
	)

	ctx, cancel := context.WithCancel(context.Background())

	return &Agent{
		config:       config,
		orchestrator: orchestrator,
		ctx:          ctx,
		cancel:       cancel,
	}, nil
}

// Start starts the server with the configured options
func (s *Agent) Start() error {
	// Validate and adjust ports if needed
	if err := s.validateAndAdjustPorts(); err != nil {
		return fmt.Errorf("port validation failed: %w", err)
	}

	// Setup shutdown handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Error channels for server goroutines
	errChan := make(chan error, 2)

	// Start gRPC server if enabled
	if s.config.GRPCPort > 0 {
		if err := s.startGRPCServer(errChan); err != nil {
			return fmt.Errorf("failed to start gRPC server: %w", err)
		}
	}

	// Start HTTP server if enabled
	if s.config.HTTPPort > 0 && s.config.GRPCPort > 0 {
		if err := s.startHTTPServer(errChan); err != nil {
			return fmt.Errorf("failed to start HTTP server: %w", err)
		}
	}

	// Start the orchestrator
	if s.config.Verbose {
		utils.LogDevloop("Config loaded successfully:")
		for _, rule := range s.orchestrator.Config.Rules {
			fmt.Printf("  Rule Name: %s\n", rule.Name)
			fmt.Printf("    Watch: %v\n", rule.Watch)
			fmt.Printf("    Commands: %v\n", rule.Commands)
		}
	}

	utils.LogDevloop("Starting orchestrator...")

	// Start orchestrator in goroutine
	go func() {
		if err := s.orchestrator.Start(); err != nil {
			errChan <- fmt.Errorf("failed to start orchestrator: %w", err)
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case <-sigChan:
		utils.LogDevloop("Received shutdown signal, shutting down...")
		s.shutdown()
		return nil
	case err := <-errChan:
		s.shutdown()
		return fmt.Errorf("server error: %w", err)
	}
}

// validateAndAdjustPorts validates port configuration and finds available ports if needed
func (s *Agent) validateAndAdjustPorts() error {
	utils.LogDevloop("Starting with ports: HTTP=%d, gRPC=%d", s.config.HTTPPort, s.config.GRPCPort)

	// Auto-discover gRPC port if requested
	if s.config.GRPCPort >= 0 {
		newGRPCPort, err := s.findAvailablePort(s.config.GRPCPort, "gRPC")
		if err != nil {
			return err
		}
		s.config.GRPCPort = newGRPCPort
	}

	// Auto-discover HTTP port if requested
	if s.config.GRPCPort > 0 && s.config.HTTPPort >= 0 {
		newHTTPPort, err := s.findAvailablePort(s.config.HTTPPort, "HTTP")
		if err != nil {
			return err
		}
		s.config.HTTPPort = newHTTPPort
	}

	return nil
}

// findAvailablePort finds an available port starting from the given port number
func (s *Agent) findAvailablePort(startPort int, portType string) (int, error) {
	// Try the original port first
	if s.isPortAvailable(startPort) {
		return startPort, nil
	}

	// If auto-ports is not enabled, return error
	if !s.config.AutoPorts {
		return 0, fmt.Errorf("port %d is in use and auto-ports is disabled", startPort)
	}

	utils.LogDevloop("Port %d is in use, searching for available %s port...", startPort, portType)

	// Search for next available port (limit search to avoid system ports)
	for port := startPort + 1; port < startPort+1000; port++ {
		if s.isPortAvailable(port) {
			utils.LogDevloop("Found available %s port: %d", portType, port)
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available %s port found in range %d-%d", portType, startPort+1, startPort+1000)
}

// isPortAvailable checks if a port is available for binding
func (s *Agent) isPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// startGRPCServer starts the gRPC server
func (s *Agent) startGRPCServer(errChan chan<- error) error {
	utils.LogDevloop("Starting gRPC server on port %d", s.config.GRPCPort)

	s.grpcServer = grpc.NewServer()
	address := fmt.Sprintf(":%d", s.config.GRPCPort)

	// Register services
	agentSvc := agent.NewAgentService(s.orchestrator)
	pb.RegisterAgentServiceServer(s.grpcServer, agentSvc)
	reflection.Register(s.grpcServer)

	l, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	// Run server in a goroutine
	go func() {
		if err := s.grpcServer.Serve(l); err != nil && err != grpc.ErrServerStopped {
			errChan <- fmt.Errorf("gRPC server failed to serve: %w", err)
		}
	}()

	return nil
}

// startHTTPServer starts the HTTP server with gRPC-gateway handlers
func (s *Agent) startHTTPServer(errChan chan<- error) error {
	utils.LogDevloop("Starting HTTP server on port %d", s.config.HTTPPort)

	// Create gRPC-gateway mux and register handlers that proxy to the gRPC server
	gwMux := runtime.NewServeMux()
	grpcEndpoint := fmt.Sprintf("localhost:%d", s.config.GRPCPort)
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if err := pb.RegisterAgentServiceHandlerFromEndpoint(s.ctx, gwMux, grpcEndpoint, opts); err != nil {
		return fmt.Errorf("failed to register gRPC-gateway handlers: %w", err)
	}

	address := fmt.Sprintf(":%d", s.config.HTTPPort)

	// Create HTTP server with middleware
	handler := s.withLogger(gwMux)
	s.httpServer = &http.Server{
		Addr:        address,
		BaseContext: func(_ net.Listener) context.Context { return s.ctx },
		Handler:     handler,
	}

	// Run server in a goroutine
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("HTTP server failed to serve: %w", err)
		}
	}()

	return nil
}

// shutdown gracefully shuts down the server
func (s *Agent) shutdown() error {
	utils.LogDevloop("Shutting down server...")

	// Cancel context first — this unblocks any in-flight gRPC-gateway connections
	s.cancel()

	// Stop HTTP server before gRPC so gateway proxy connections drain
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			utils.LogDevloop("HTTP server shutdown error: %v", err)
		}
	}

	// Stop gRPC server — GracefulStop waits for active RPCs, so use a timeout fallback
	if s.grpcServer != nil {
		done := make(chan struct{})
		go func() {
			s.grpcServer.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			utils.LogDevloop("gRPC graceful stop timed out, forcing stop")
			s.grpcServer.Stop()
		}
	}

	// Stop orchestrator (stops rules, then LogManager/broadcasters)
	if s.orchestrator != nil {
		s.orchestrator.Stop()
	}

	// Cancel context
	s.cancel()

	utils.LogDevloop("Server shutdown complete")
	return nil
}

// withLogger adds request logging middleware
func (s *Agent) withLogger(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// pass the handler to httpsnoop to get http status and latency
		m := httpsnoop.CaptureMetrics(handler, writer, request)
		// printing extracted data (only log non-200 responses to reduce noise)
		if m.Code != 200 {
			utils.LogDevloop("http[%d] %s %s, Query: %s, Duration: %s",
				m.Code, request.Method, request.URL.Path, request.URL.RawQuery, m.Duration)
		}
	})
}
