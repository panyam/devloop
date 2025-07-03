package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"example.com/microservices/shared"
)

var startTime = time.Now()

// Service endpoints
const (
	authServiceURL = "http://localhost:8081"
	userServiceURL = "http://localhost:8082"
)

// createReverseProxy creates a reverse proxy for a service
func createReverseProxy(targetURL string) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}
	
	proxy := httputil.NewSingleHostReverseProxy(target)
	
	// Custom error handler
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[gateway] Proxy error for %s: %v", r.URL.Path, err)
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(shared.ErrorResponse{
			Error:   "service_unavailable",
			Message: fmt.Sprintf("Service temporarily unavailable: %v", err),
			Code:    502,
		})
	}
	
	return proxy, nil
}

func main() {
	// Create reverse proxies
	authProxy, err := createReverseProxy(authServiceURL)
	if err != nil {
		log.Fatalf("Failed to create auth proxy: %v", err)
	}
	
	userProxy, err := createReverseProxy(userServiceURL)
	if err != nil {
		log.Fatalf("Failed to create user proxy: %v", err)
	}

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Check health of all services
		services := map[string]string{
			"auth": authServiceURL + "/health",
			"user": userServiceURL + "/health",
		}
		
		allHealthy := true
		serviceStatuses := make(map[string]string)
		
		for name, url := range services {
			resp, err := http.Get(url)
			if err != nil || resp.StatusCode != http.StatusOK {
				allHealthy = false
				serviceStatuses[name] = "unhealthy"
			} else {
				serviceStatuses[name] = "healthy"
				resp.Body.Close()
			}
		}
		
		status := "healthy"
		if !allHealthy {
			status = "degraded"
		}
		
		health := map[string]interface{}{
			"service":  "api-gateway",
			"status":   status,
			"uptime":   time.Since(startTime).String(),
			"time":     time.Now(),
			"services": serviceStatuses,
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(health)
	})

	// Auth routes - proxy to auth service
	http.HandleFunc("/auth/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[gateway] Proxying %s %s to auth service", r.Method, r.URL.Path)
		
		// For login requests, intercept and log
		if r.URL.Path == "/auth/login" && r.Method == http.MethodPost {
			// Read body to log username (but still forward it)
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(body))
			
			var req shared.AuthRequest
			if json.Unmarshal(body, &req) == nil {
				log.Printf("[gateway] Login attempt for user: %s", req.Username)
			}
		}
		
		authProxy.ServeHTTP(w, r)
	})

	// User routes - proxy to user service
	http.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[gateway] Proxying %s %s to user service", r.Method, r.URL.Path)
		userProxy.ServeHTTP(w, r)
	})
	
	http.HandleFunc("/users/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[gateway] Proxying %s %s to user service", r.Method, r.URL.Path)
		userProxy.ServeHTTP(w, r)
	})

	// Root endpoint
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(shared.ErrorResponse{
				Error:   "not_found",
				Message: "Endpoint not found",
				Code:    404,
			})
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"service": "api-gateway",
			"version": "1.0.0",
			"endpoints": map[string]string{
				"health":      "GET /health",
				"auth_login":  "POST /auth/login",
				"auth_verify": "GET /auth/verify",
				"users_list":  "GET /users",
				"users_get":   "GET /users/{id}",
				"users_update": "PUT /users/update",
			},
		})
	})

	// Add CORS middleware for all routes
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		http.DefaultServeMux.ServeHTTP(w, r)
	})

	log.Println("[gateway] API Gateway starting on :8080")
	log.Println("[gateway] Routes: /auth/* → Auth Service, /users/* → User Service")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal(err)
	}
}