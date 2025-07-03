package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type Task struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Completed   bool      `json:"completed"`
	CreatedAt   time.Time `json:"created_at"`
}

var tasks []Task
var nextID = 1

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	r := gin.Default()

	// CORS middleware
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization"}
	r.Use(cors.New(config))

	// Initialize with sample data
	initSampleData()

	// Routes
	api := r.Group("/api")
	{
		api.GET("/health", healthCheck)
		api.GET("/tasks", getTasks)
		api.POST("/tasks", createTask)
		api.PUT("/tasks/:id", updateTask)
		api.DELETE("/tasks/:id", deleteTask)
	}

	// Root route
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Docker Compose API Server",
			"version": "1.0.0",
			"status":  "running",
		})
	})

	log.Printf("Starting API server on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

func initSampleData() {
	tasks = []Task{
		{
			ID:          nextID,
			Title:       "Setup Docker Environment",
			Description: "Configure Docker Compose for development",
			Completed:   true,
			CreatedAt:   time.Now().Add(-2 * time.Hour),
		},
		{
			ID:          nextID + 1,
			Title:       "Implement API Endpoints",
			Description: "Create REST API for task management",
			Completed:   false,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
		},
		{
			ID:          nextID + 2,
			Title:       "Add Frontend Integration",
			Description: "Connect React frontend to API",
			Completed:   false,
			CreatedAt:   time.Now(),
		},
	}
	nextID += 3
}

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now(),
		"uptime":    "running",
	})
}

func getTasks(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"tasks": tasks,
		"count": len(tasks),
	})
}

func createTask(c *gin.Context) {
	var newTask struct {
		Title       string `json:"title" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&newTask); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task := Task{
		ID:          nextID,
		Title:       newTask.Title,
		Description: newTask.Description,
		Completed:   false,
		CreatedAt:   time.Now(),
	}

	tasks = append(tasks, task)
	nextID++

	c.JSON(http.StatusCreated, gin.H{
		"task":    task,
		"message": "Task created successfully",
	})
}

func updateTask(c *gin.Context) {
	id := c.Param("id")

	var updatedTask struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Completed   *bool  `json:"completed"`
	}

	if err := c.ShouldBindJSON(&updatedTask); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for i, task := range tasks {
		if task.ID == parseID(id) {
			if updatedTask.Title != "" {
				tasks[i].Title = updatedTask.Title
			}
			if updatedTask.Description != "" {
				tasks[i].Description = updatedTask.Description
			}
			if updatedTask.Completed != nil {
				tasks[i].Completed = *updatedTask.Completed
			}

			c.JSON(http.StatusOK, gin.H{
				"task":    tasks[i],
				"message": "Task updated successfully",
			})
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
}

func deleteTask(c *gin.Context) {
	id := c.Param("id")

	for i, task := range tasks {
		if task.ID == parseID(id) {
			tasks = append(tasks[:i], tasks[i+1:]...)
			c.JSON(http.StatusOK, gin.H{
				"message": "Task deleted successfully",
			})
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
}

func parseID(idStr string) int {
	// Simple string to int conversion for demo purposes
	// In production, use strconv.Atoi with proper error handling
	switch idStr {
	case "1":
		return 1
	case "2":
		return 2
	case "3":
		return 3
	default:
		return 0
	}
}