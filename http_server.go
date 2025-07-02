package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
)

const DEFAULT_HTTP_PORT = "9999"

// HTTPServer provides an HTTP endpoint for streaming rule logs.
type HTTPServer struct {
	server       *http.Server
	orchestrator *Orchestrator // Reference to the orchestrator
	logManager   *LogManager
	port         string
}

// NewHTTPServer creates a new HTTPServer instance.
func NewHTTPServer(orchestrator *Orchestrator, logManager *LogManager, port string) *HTTPServer {
	if port == "" {
		return nil // Do not create server if port is empty
	}

	if port == "default" {
		port = DEFAULT_HTTP_PORT
	}
	r := mux.NewRouter()
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	hs := &HTTPServer{
		server:       server,
		orchestrator: orchestrator,
		logManager:   logManager,
		port:         port,
	}

	r.HandleFunc("/stream/{ruleName}", hs.streamLogsHandler).Methods("GET")
	r.HandleFunc("/config", hs.handleGetConfig).Methods("GET")
	r.HandleFunc("/status/{ruleName}", hs.handleGetRuleStatus).Methods("GET")
	r.HandleFunc("/trigger/{ruleName}", hs.handleTriggerRule).Methods("POST")
	r.HandleFunc("/watched-paths", hs.handleListWatchedPaths).Methods("GET")
	r.HandleFunc("/file-content", hs.handleReadFileContent).Methods("GET")

	return hs
}

// handleListWatchedPaths returns a list of all paths being watched by devloop.
func (hs *HTTPServer) handleListWatchedPaths(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	paths := hs.orchestrator.GetWatchedPaths()
	json.NewEncoder(w).Encode(paths)
}

// handleReadFileContent reads and returns the content of a specified file.
func (hs *HTTPServer) handleReadFileContent(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "Missing 'path' query parameter", http.StatusBadRequest)
		return
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read file %q: %v", filePath, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write(content)
}

// handleTriggerRule triggers the execution of a specific rule.
func (hs *HTTPServer) handleTriggerRule(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ruleName := vars["ruleName"]

	err := hs.orchestrator.TriggerRule(ruleName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Rule %q triggered successfully", ruleName)
}

// handleGetConfig returns the entire devloop configuration.
func (hs *HTTPServer) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hs.orchestrator.Config)
}

// handleGetRuleStatus returns the current status of a specific rule.
func (hs *HTTPServer) handleGetRuleStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ruleName := vars["ruleName"]

	status, ok := hs.orchestrator.GetRuleStatus(ruleName)
	if !ok {
		http.Error(w, "Rule not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// Start begins listening for HTTP requests.
func (hs *HTTPServer) Start() {
	log.Printf("[devloop] Starting HTTP server on port %s for log streaming...", hs.port)
	go func() {
		if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[devloop] HTTP server failed: %v", err)
		}
	}()
}

// Stop gracefully shuts down the HTTP server.
func (hs *HTTPServer) Stop() error {
	log.Println("[devloop] Stopping HTTP server...")
	return hs.server.Close()
}

// streamLogsHandler handles requests for streaming rule logs via Server-Sent Events (SSE).
func (hs *HTTPServer) streamLogsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ruleName := vars["ruleName"]
	filter := r.URL.Query().Get("filter")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*") // Allow CORS for broader access

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	log.Printf("[devloop] Client connected to stream for rule %q (filter: %q)", ruleName, filter)

	// Send an initial comment to keep the connection alive and signal readiness
	if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
		log.Printf("[devloop] Error sending initial ping for rule %q: %v", ruleName, err)
		return
	}
	flusher.Flush()

	// Stream logs from the LogManager
	err := hs.logManager.StreamLogs(ruleName, filter, w)
	if err != nil {
		log.Printf("[devloop] Error streaming logs for rule %q: %v", ruleName, err)
		// Do not send HTTP error if headers already sent
		return
	}

	flusher.Flush()
	log.Printf("[devloop] Client disconnected from stream for rule %q", ruleName)
}
