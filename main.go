// Package main provides the devloop command-line interface for development workflow automation.
//
// Devloop is a versatile tool designed to streamline your inner development loop,
// particularly within Multi-Component Projects. It combines functionalities inspired
// by live-reloading tools like air (for Go) and build automation tools like make,
// focusing on simple, configuration-driven orchestration of tasks based on file system changes.
//
// # Installation
//
//	go install github.com/panyam/devloop@latest
//
// # Quick Start
//
// Create a .devloop.yaml file:
//
//	rules:
//	  - name: "Go Backend Build and Run"
//	    watch:
//	      - action: "include"
//	        patterns:
//	          - "**/*.go"
//	          - "go.mod"
//	          - "go.sum"
//	    commands:
//	      - "go build -o ./bin/server ./cmd/server"
//	      - "./bin/server"
//
// Run devloop:
//
//	devloop -c .devloop.yaml
//
// # Architecture Modes
//
// Devloop operates in two distinct modes:
//
// 1. Agent Mode (Default): Local file watcher for single project development.
// 2. Gateway Mode: Where the devloop server acts as a gateway and reverse proxy for mulitple devloop instances
//
// When running in Agent mode it can expose http and/or grpc services for API access.  It can also connecto a devloop
// gateway for reverse proxy API access
//
// # Examples
//
// See the examples/ directory for comprehensive real-world examples including:
// - Full-stack web applications
// - Microservices architectures
// - Data science workflows
// - Docker compose setups
// - Frontend framework development
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/felixge/httpsnoop"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/mark3labs/mcp-go/server"
	"github.com/panyam/devloop/agent"
	"github.com/panyam/devloop/gateway"
	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"github.com/panyam/devloop/gen/go/devloop/v1/v1mcp"
	"github.com/panyam/devloop/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

//go:embed VERSION
var version string

var verbose bool

// findAvailablePort finds an available port starting from the given port number
func findAvailablePort(startPort int, portType string) (int, error) {
	// Try the original port first
	if isPortAvailable(startPort) {
		return startPort, nil
	}

	utils.LogDevloop("Port %d is in use, searching for available %s port...", startPort, portType)

	// Search for next available port (limit search to avoid system ports)
	for port := startPort + 1; port < startPort+1000; port++ {
		if isPortAvailable(port) {
			utils.LogDevloop("Found available %s port: %d", portType, port)
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available %s port found in range %d-%d", portType, startPort+1, startPort+1000)
}

// isPortAvailable checks if a port is available for binding
func isPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// validateFlags checks flag combinations and warns about ignored flags
func validateFlags(httpPort, grpcPort *int) {
	log.Printf("[devloop] Starting")

	// Auto-discover ports if requested
	if *grpcPort >= 0 {
		if newGrpcPort, err := findAvailablePort(*grpcPort, "gRPC"); err != nil {
			log.Fatalf("Error: %v\n", err)
		} else {
			*grpcPort = newGrpcPort
		}
	}

	if *grpcPort > 0 && *httpPort >= 0 {
		if newHttpPort, err := findAvailablePort(*httpPort, "HTTP"); err != nil {
			log.Fatalf("Error: %v\n", err)
		} else {
			*httpPort = newHttpPort
		}
	}
}

func main() {
	var configPath string
	var showVersion bool
	var httpPort int
	var grpcPort int
	var gatewayAddress string
	var mode string
	var enableMCP bool

	// Define global flags
	flag.StringVar(&configPath, "c", "./.devloop.yaml", "Path to the .devloop.yaml configuration file")
	flag.BoolVar(&showVersion, "version", false, "Display version information")
	flag.BoolVar(&verbose, "v", false, "Enable verbose logging")
	flag.IntVar(&grpcPort, "grpc-port", 50051, "Port for the gRPC server.  If the port is not specified (ie -1) then the gRPC server will not be started.  If port is 0 then an available port is found")
	flag.IntVar(&httpPort, "http-port", 9999, "Port for the HTTP gateway server.  If the port is not specified (ie -1) or the grpc port is not specified then the http gateway will not be started.  If port is 0 then an available port is found.")
	flag.StringVar(&gatewayAddress, "gateway", "", "Host and port of the gateway to connect to to expose the agent.")
	flag.BoolVar(&enableMCP, "enable-mcp", true, "Enable MCP server for AI tool integration.  The grpc sever MUST be started for MCP to be active")
	flag.StringVar(&mode, "mode", "standalone", "Operating mode: standalone, agent, or gateway")

	// Define subcommands
	convertCmd := flag.NewFlagSet("convert", flag.ExitOnError)
	convertInputPath := convertCmd.String("i", ".air.toml", "Path to the .air.toml input file")

	// Parse global flags first
	flag.Parse()

	if showVersion {
		fmt.Printf("devloop version %s\n", version)
		return
	}

	// Check for subcommands
	if len(flag.Args()) > 0 {
		switch flag.Args()[0] {
		case "convert":
			convertCmd.Parse(flag.Args()[1:])
			if err := gateway.ConvertAirToml(*convertInputPath); err != nil {
				log.Fatalf("Error: Failed to convert .air.toml: %v\n", err)
			}
			return
		}
	}

	// Run the orchestrator in the selected mode
	orchestrator := newOrchestrator(configPath)

	// Validate flag combinations and warn about ignored flags
	validateFlags(&httpPort, &grpcPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srvErr := make(chan error)
	grpcStopChan := make(chan bool, 1)
	httpStopChan := make(chan bool, 1)
	defer func() {
		close(srvErr)
		close(grpcStopChan)
		close(httpStopChan)
	}()

	if grpcPort > 0 {
		startGrpcServer(orchestrator, ctx, grpcPort, srvErr, grpcStopChan)
	}

	if httpPort > 0 {
		startHttpServer(orchestrator, ctx, httpPort, srvErr, httpStopChan, enableMCP)
	}

	// , mode, httpPort, grpcPort, enableMCP

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	shutdownComplete := make(chan bool, 1)
	go func() {
		<-sigChan
		utils.LogDevloop("Shutting down...")
		if grpcStopChan != nil {
			grpcStopChan <- true
		}
		if httpStopChan != nil {
			httpStopChan <- true
		}
		log.Println("Here 1....")
		orchestrator.Stop()
		log.Println("Here 2....")
		shutdownComplete <- true
	}()

	if verbose {
		utils.LogDevloop("Config loaded successfully:")
		for _, rule := range orchestrator.Config.Rules {
			fmt.Printf("  Rule Name: %s\n", rule.Name)
			fmt.Printf("    Watch: %v\n", rule.Matchers)
			fmt.Printf("    Commands: %v\n", rule.Commands)
		}
	}

	utils.LogDevloop("Starting orchestrator...")
	if err := orchestrator.Start(); err != nil {
		log.Fatalf("Error: Failed to start orchestrator: %v\n", err)
	}

	// Wait for shutdown to complete if it was triggered
	select {
	case <-shutdownComplete:
		utils.LogDevloop("Shutdown complete")
		os.Exit(0)
	default:
		// Normal exit
	}
}

func newOrchestrator(configPath string) *agent.Orchestrator {
	orchestrator, err := agent.NewOrchestrator(configPath)
	if err != nil {
		log.Fatalf("Error: Failed to initialize orchestrator: %v\n", err)
	}
	orchestrator.Verbose = verbose

	// Initialize global logger for consistent formatting across all components
	utils.InitGlobalLogger(
		orchestrator.Config.Settings.PrefixLogs,
		int(orchestrator.Config.Settings.PrefixMaxLength),
		orchestrator.ColorManager,
	)
	return orchestrator
}

// Start HTTP/gRPC gateway service (standalone and gateway modes only)
/*
	if mode == "standalone" || mode == "gateway" {
		utils.LogDevloop("Starting HTTP/gRPC gateway service...")
		gatewayService = gateway.NewGatewayService(orchestrator, mode)
		err = gatewayService.Start(grpcPort, httpPort)
		if err != nil {
			log.Fatalf("Error: Failed to start gateway service: %v\n", err)
		}
		utils.LogDevloop("Gateway service started: HTTP=:%d, gRPC=:%d", httpPort, grpcPort)

		// Mount MCP handlers on the gateway's HTTP server if enabled
		if enableMCP {
			utils.LogDevloop("Mounting MCP handlers on gateway HTTP server...")
			mcpService = mcp.NewMCPService(orchestrator)
			mcpHandler, err := mcpService.CreateHandler()
			if err != nil {
				log.Fatalf("Error: Failed to create MCP handler: %v\n", err)
			}
			gatewayService.MountMCPHandlers(mcpHandler)
			utils.LogDevloop("MCP handlers mounted at http://localhost:%d/mcp/", httpPort)
		}
	} else {
		utils.LogDevloop("Skipping HTTP/gRPC gateway service (not needed in %s mode)", mode)
		if enableMCP {
			utils.LogDevloop("WARNING: MCP requires HTTP server (only available in standalone/gateway modes)")
		}
	}
*/

func startGrpcServer(orchestrator *agent.Orchestrator, _ context.Context, grpcPort int, srvErr chan error, srvChan chan bool) error {
	utils.LogDevloop("Starting GRPC Server on port %d", grpcPort)
	server := grpc.NewServer(
	// grpc.UnaryInterceptor(EnsureAccessToken), // Add interceptors if needed
	)

	// Register services
	address := fmt.Sprintf(":%d", grpcPort)
	agentSvc := agent.NewAgentService(orchestrator)
	pb.RegisterAgentServiceServer(server, agentSvc)
	l, err := net.Listen("tcp", address)
	if err != nil {
		slog.Error("error in listening on port", "port", address, "err", err)
		// Consider returning the error instead of fatal/panic in Start method
		return fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	// the gRPC server
	reflection.Register(server)

	// Run server in a goroutine to allow graceful shutdown
	go func() {
		if err := server.Serve(l); err != nil && err != grpc.ErrServerStopped {
			log.Printf("grpc server failed to serve: %v", err)
			srvErr <- err // Send error to the main app routine
		}
	}()

	// Handle shutdown signal
	go func() {
		slog.Info("Did we come here??")
		<-srvChan // Wait for shutdown signal from main app
		slog.Info("Shutting down gRPC server...")
		server.GracefulStop()
		slog.Info("gRPC server stopped.")
	}()

	return nil // Indicate successful start
}

func startHttpServer(orchestrator *agent.Orchestrator, ctx context.Context, httpPort int, srvErr chan error, stopChan chan bool, enableMCP bool) error {
	utils.LogDevloop("Starting HTTP GRPC Gateway Server on port %d", httpPort)
	gwmux := runtime.NewServeMux()
	address := fmt.Sprintf(":%d", httpPort)
	err := pb.RegisterAgentServiceHandlerFromEndpoint(ctx, gwmux, address, []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())})
	if err != nil {
		return fmt.Errorf("[gateway] Failed to register gateway: %v", err)
	}

	// Create main HTTP router with organized path prefixes
	mainMux := http.NewServeMux()

	// Mount gRPC gateway at /api prefix
	mainMux.Handle("/api/", http.StripPrefix("/api", gwmux))

	if enableMCP {
		mcpServer := server.NewMCPServer("devloop", "1.0.0")
		adapter := agent.NewAgentService(orchestrator)
		v1mcp.RegisterAgentServiceHandler(mcpServer, adapter)
		mcpHandler := server.NewStreamableHTTPServer(mcpServer,
			server.WithStateLess(true),      // Stateless mode - no sessionId required
			server.WithEndpointPath("/mcp"), // Single endpoint for all MCP operations
		)
		mainMux.Handle("/mcp/", http.StripPrefix("/mcp", mcpHandler))
		utils.LogDevloop("MCP enabled on /mcp")
	}

	// handler := otelhttp.NewHandler(mux, "gateway", otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string { return fmt.Sprintf("%s %s %s", operation, r.Method, r.URL.Path) }))
	handler := withLogger(mainMux)
	// if s.AllowLocalDev { handler = CORS(handler) }
	server := &http.Server{
		Addr:        address,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
		Handler:     handler,
	}

	go func() {
		<-stopChan
		if err := server.Shutdown(context.Background()); err != nil {
			log.Fatalln(err)
			panic(err)
		}
	}()
	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Printf("http server failed to serve: %v", err)
			srvErr <- err // Send error to the main app routine
		}
	}()
	return nil
}

func withLogger(handler http.Handler) http.Handler {
	// the create a handler
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// pass the handler to httpsnoop to get http status and latency
		m := httpsnoop.CaptureMetrics(handler, writer, request)
		// printing exracted data
		if false && m.Code != 200 { // turn off frequent logs
			log.Printf("http[%d]-- %s -- %s, Query: %s\n", m.Code, m.Duration, request.URL.Path, request.URL.RawQuery)
		}
	})
}

func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// fmt.Println(r.Header)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, Origin, Cache-Control, X-Requested-With")
		//w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Methods", "PUT, DELETE")

		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}

		next.ServeHTTP(w, r)
	})
}
