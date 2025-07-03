package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

type Todo struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"created_at"`
}

type TodoStore struct {
	mu    sync.RWMutex
	todos map[string]*Todo
}

func NewTodoStore() *TodoStore {
	return &TodoStore{
		todos: make(map[string]*Todo),
	}
}

func (s *TodoStore) GetAll() []*Todo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	todos := make([]*Todo, 0, len(s.todos))
	for _, todo := range s.todos {
		todos = append(todos, todo)
	}
	return todos
}

func (s *TodoStore) Create(title string) *Todo {
	s.mu.Lock()
	defer s.mu.Unlock()

	todo := &Todo{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Title:     title,
		Completed: false,
		CreatedAt: time.Now(),
	}
	s.todos[todo.ID] = todo
	return todo
}

func (s *TodoStore) Update(id string, completed bool) (*Todo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	todo, exists := s.todos[id]
	if !exists {
		return nil, false
	}
	todo.Completed = completed
	return todo, true
}

func main() {
	store := NewTodoStore()

	// Enable CORS for frontend
	corsMiddleware := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next(w, r)
		}
	}

	// GET /api/todos
	http.HandleFunc("/api/todos", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		todos := store.GetAll()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(todos)
	}))

	// POST /api/todos
	http.HandleFunc("/api/todos/create", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		todo := store.Create(req.Title)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(todo)
	}))

	// PUT /api/todos/:id
	http.HandleFunc("/api/todos/update", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			ID        string `json:"id"`
			Completed bool   `json:"completed"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		todo, ok := store.Update(req.ID, req.Completed)
		if !ok {
			http.Error(w, "Todo not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(todo)
	}))

	log.Println("Starting server on http://localhost:20202")
	if err := http.ListenAndServe(":20202", nil); err != nil {
		log.Fatal(err)
	}
}
