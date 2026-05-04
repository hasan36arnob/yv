package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

//go:embed index.html style.css script.js
var staticFiles embed.FS

type Task struct {
	ID        int64  `json:"id"`
	Text      string `json:"text"`
	Completed bool   `json:"completed"`
	CreatedAt string `json:"createdAt"`
}

type Store struct {
	Tasks []Task `json:"tasks"`
	mu    sync.RWMutex
}

var (
	dataFile string
	store    *Store
)

func init() {
	dataFile = os.Getenv("DATA_FILE")
	if dataFile == "" {
		dataFile = "tasks.json"
	}
	store = &Store{Tasks: []Task{}}
	loadTasks()
}

func loadTasks() {
	data, err := ioutil.ReadFile(dataFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Println("Error reading tasks file:", err)
		}
		store.Tasks = []Task{}
		return
	}

	var data_ struct {
		Tasks []Task `json:"tasks"`
	}
	err = json.Unmarshal(data, &data_)
	if err != nil {
		log.Println("Error unmarshaling tasks:", err)
		store.Tasks = []Task{}
		return
	}
	store.Tasks = data_.Tasks
	log.Println("Loaded", len(store.Tasks), "tasks from persistent storage")
}

func saveTasks() error {
	store.mu.RLock()
	defer store.mu.RUnlock()

	data := map[string][]Task{"tasks": store.Tasks}
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(dataFile, jsonData, 0644)
}

func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func handleOptions(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	w.WriteHeader(http.StatusOK)
}

func getTasks(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		handleOptions(w, r)
		return
	}

	store.mu.RLock()
	tasks := store.Tasks
	store.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

func createTask(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		handleOptions(w, r)
		return
	}

	var task Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, `{"error":"Invalid request"}`, http.StatusBadRequest)
		return
	}

	if task.Text == "" {
		http.Error(w, `{"error":"Task text is required"}`, http.StatusBadRequest)
		return
	}

	store.mu.Lock()
	newTask := Task{
		ID:        time.Now().UnixNano(),
		Text:      task.Text,
		Completed: false,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	store.Tasks = append([]Task{newTask}, store.Tasks...)
	store.mu.Unlock()

	saveTasks()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(newTask)
}

func updateTask(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		handleOptions(w, r)
		return
	}

	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"Invalid task ID"}`, http.StatusBadRequest)
		return
	}

	var task Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, `{"error":"Invalid request"}`, http.StatusBadRequest)
		return
	}

	store.mu.Lock()
	found := false
	for i, t := range store.Tasks {
		if t.ID == id {
			store.Tasks[i].Text = task.Text
			store.Tasks[i].Completed = task.Completed
			found = true
			break
		}
	}
	store.mu.Unlock()

	if !found {
		http.Error(w, `{"error":"Task not found"}`, http.StatusNotFound)
		return
	}

	saveTasks()

	w.Header().Set("Content-Type", "application/json")
	task.ID = id
	json.NewEncoder(w).Encode(task)
}

func deleteTask(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		handleOptions(w, r)
		return
	}

	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"Invalid task ID"}`, http.StatusBadRequest)
		return
	}

	store.mu.Lock()
	found := false
	for i, t := range store.Tasks {
		if t.ID == id {
			store.Tasks = append(store.Tasks[:i], store.Tasks[i+1:]...)
			found = true
			break
		}
	}
	store.mu.Unlock()

	if !found {
		http.Error(w, `{"error":"Task not found"}`, http.StatusNotFound)
		return
	}

	saveTasks()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Task deleted successfully"})
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"storage":   "JSON file-based",
	})
}

func main() {
	http.HandleFunc("/api/tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			getTasks(w, r)
		} else if r.Method == "POST" {
			createTask(w, r)
		} else if r.Method == "OPTIONS" {
			handleOptions(w, r)
		} else {
			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/api/tasks/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			updateTask(w, r)
		} else if r.Method == "OPTIONS" {
			handleOptions(w, r)
		} else {
			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/api/tasks/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			deleteTask(w, r)
		} else if r.Method == "OPTIONS" {
			handleOptions(w, r)
		} else {
			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/health", healthCheck)
	http.Handle("/", http.FileServer(http.FS(staticFiles)))

	port := ":5000"
	fmt.Printf("\n════════════════════════════════════════════════════════════\n")
	fmt.Printf("  TodoPro API Server - Production Ready\n")
	fmt.Printf("════════════════════════════════════════════════════════════\n\n")
	fmt.Printf("📍 Running on http://localhost%s\n\n", port)
	fmt.Printf("🔌 API Endpoints:\n")
	fmt.Printf("   GET    /api/tasks         → Fetch all tasks\n")
	fmt.Printf("   POST   /api/tasks         → Create a new task\n")
	fmt.Printf("   PUT    /api/tasks/update  → Update task (with ?id=taskId)\n")
	fmt.Printf("   DELETE /api/tasks/delete  → Delete task (with ?id=taskId)\n")
	fmt.Printf("   GET    /health           → Health check\n\n")
	fmt.Printf("💾 Storage: JSON file-based (tasks.json)\n")
	fmt.Printf("🌐 CORS: Enabled for all origins\n")
	fmt.Printf("🔒 Thread-safe with mutex locking\n\n")
	fmt.Printf("════════════════════════════════════════════════════════════\n\n")

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
