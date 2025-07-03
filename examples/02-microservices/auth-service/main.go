package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"example.com/microservices/shared"
)

var startTime = time.Now()

// Simple in-memory token store (in production, use Redis or similar)
var tokens = make(map[string]*shared.User)

// Demo users (in production, use a database)
var users = map[string]string{
	"demo":  "demo123",
	"admin": "admin123",
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func main() {
	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		health := shared.HealthResponse{
			Service: "auth-service",
			Status:  "healthy",
			Uptime:  time.Since(startTime).String(),
			Time:    time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(health)
	})

	// Login endpoint
	http.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req shared.AuthRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(shared.ErrorResponse{
				Error:   "invalid_request",
				Message: "Invalid request body",
				Code:    400,
			})
			return
		}

		// Validate credentials
		if pass, ok := users[req.Username]; !ok || pass != req.Password {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(shared.ErrorResponse{
				Error:   "invalid_credentials",
				Message: "Invalid username or password",
				Code:    401,
			})
			return
		}

		// Generate token
		token := generateToken()
		user := &shared.User{
			ID:        fmt.Sprintf("user-%d", time.Now().Unix()),
			Username:  req.Username,
			Email:     fmt.Sprintf("%s@example.com", req.Username),
			FullName:  strings.Title(req.Username) + " User",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		tokens[token] = user

		// Return auth response
		resp := shared.AuthResponse{
			Token:     token,
			ExpiresAt: time.Now().Add(24 * time.Hour),
			User:      *user,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		log.Printf("[auth] User %s logged in successfully", req.Username)
	})

	// Verify token endpoint (for other services)
	http.HandleFunc("/auth/verify", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(shared.ErrorResponse{
				Error:   "missing_token",
				Message: "Authorization header required",
				Code:    401,
			})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		user, ok := tokens[token]
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(shared.ErrorResponse{
				Error:   "invalid_token",
				Message: "Invalid or expired token",
				Code:    401,
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	})

	log.Println("[auth] Auth service starting on :8081")
	if err := http.ListenAndServe(":8081", nil); err != nil {
		log.Fatal(err)
	}
}