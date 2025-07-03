package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"example.com/microservices/shared"
)

var startTime = time.Now()

// Simple in-memory user store
type UserStore struct {
	mu    sync.RWMutex
	users map[string]*shared.User
}

var store = &UserStore{
	users: make(map[string]*shared.User),
}

// verifyToken checks with auth service if token is valid
func verifyToken(token string) (*shared.User, error) {
	req, err := http.NewRequest("GET", "http://localhost:20202/auth/verify", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth service unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("invalid token: %s", body)
	}

	var user shared.User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return &user, nil
}

// authMiddleware validates tokens for protected endpoints
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		user, err := verifyToken(token)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(shared.ErrorResponse{
				Error:   "invalid_token",
				Message: err.Error(),
				Code:    401,
			})
			return
		}

		// Store user in memory
		store.mu.Lock()
		store.users[user.ID] = user
		store.mu.Unlock()

		next(w, r)
	}
}

func main() {
	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		health := shared.HealthResponse{
			Service: "user-service",
			Status:  "healthy",
			Uptime:  time.Since(startTime).String(),
			Time:    time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(health)
	})

	// Get user by ID (protected)
	http.HandleFunc("/users/", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID := strings.TrimPrefix(r.URL.Path, "/users/")
		if userID == "" {
			// Return all users
			store.mu.RLock()
			users := make([]*shared.User, 0, len(store.users))
			for _, user := range store.users {
				users = append(users, user)
			}
			store.mu.RUnlock()

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(users)
			return
		}

		// Return specific user
		store.mu.RLock()
		user, exists := store.users[userID]
		store.mu.RUnlock()

		if !exists {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(shared.ErrorResponse{
				Error:   "user_not_found",
				Message: fmt.Sprintf("User %s not found", userID),
				Code:    404,
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
		log.Printf("[user] Retrieved user %s", userID)
	}))

	// Update user (protected)
	http.HandleFunc("/users/update", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(shared.ErrorResponse{
				Error:   "invalid_request",
				Message: "Invalid request body",
				Code:    400,
			})
			return
		}

		userID, ok := updates["id"].(string)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(shared.ErrorResponse{
				Error:   "missing_id",
				Message: "User ID required",
				Code:    400,
			})
			return
		}

		store.mu.Lock()
		user, exists := store.users[userID]
		if exists {
			// Simple update logic - in production, be more careful
			if email, ok := updates["email"].(string); ok {
				user.Email = email
			}
			if fullName, ok := updates["full_name"].(string); ok {
				user.FullName = fullName
			}
			user.UpdatedAt = time.Now()
		}
		store.mu.Unlock()

		if !exists {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(shared.ErrorResponse{
				Error:   "user_not_found",
				Message: fmt.Sprintf("User %s not found", userID),
				Code:    404,
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
		log.Printf("[user] Updated user %s", userID)
	}))

	log.Println("[user] User service starting on :20203")
	if err := http.ListenAndServe(":20203", nil); err != nil {
		log.Fatal(err)
	}
}
