package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

//go:embed index.html style.css script.js
var staticFiles embed.FS

// Database models
type User struct {
	ID                uint      `json:"id" gorm:"primaryKey"`
	Email             string    `json:"email" gorm:"uniqueIndex;not null"`
	PasswordHash      string    `json:"-" gorm:"not null"`
	FirstName         string    `json:"first_name"`
	LastName          string    `json:"last_name"`
	Role              string    `json:"role" gorm:"default:'member'"` // owner, admin, member
	SubscriptionPlan  string    `json:"subscription_plan" gorm:"default:'free'"` // free, personal, pro, team
	SubscriptionID    string    `json:"stripe_subscription_id"`
	CustomerID        string    `json:"stripe_customer_id"`
	IsActive          bool      `json:"is_active" gorm:"default:true"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	Workspaces        []Workspace `json:"workspaces,omitempty" gorm:"foreignKey:OwnerID"`
	Tasks             []Task     `json:"tasks,omitempty" gorm:"foreignKey:AssigneeID"`
}

type Workspace struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"not null"`
	Description string    `json:"description"`
	OwnerID     uint      `json:"owner_id" gorm:"not null"`
	Owner       User      `json:"owner,omitempty" gorm:"foreignKey:OwnerID"`
	IsActive    bool      `json:"is_active" gorm:"default:true"`
	PlanTier    string    `json:"plan_tier" gorm:"default:'free'"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	TeamMembers []TeamMember `json:"team_members,omitempty" gorm:"foreignKey:WorkspaceID"`
	Tasks       []Task       `json:"tasks,omitempty" gorm:"foreignKey:WorkspaceID"`
}

type TeamMember struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	WorkspaceID uint      `json:"workspace_id" gorm:"not null"`
	Workspace   Workspace `json:"workspace,omitempty" gorm:"foreignKey:WorkspaceID"`
	UserID      uint      `json:"user_id" gorm:"not null"`
	User        User      `json:"user,omitempty" gorm:"foreignKey:UserID"`
	Role        string    `json:"role" gorm:"default:'member'"` // owner, admin, member, viewer
	IsActive    bool      `json:"is_active" gorm:"default:true"`
	JoinedAt    time.Time `json:"joined_at"`
	CreatedAt   time.Time `json:"created_at"`
}

type Task struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	WorkspaceID uint      `json:"workspace_id" gorm:"not null"`
	Workspace   Workspace `json:"workspace,omitempty" gorm:"foreignKey:WorkspaceID"`
	Title       string    `json:"title" gorm:"not null"`
	Description string    `json:"description"`
	Status      string    `json:"status" gorm:"default:'pending'"` // pending, in_progress, completed
	Priority    string    `json:"priority" gorm:"default:'medium'"` // low, medium, high, urgent
	AssigneeID  *uint     `json:"assignee_id"`
	Assignee    *User     `json:"assignee,omitempty" gorm:"foreignKey:AssigneeID"`
	DueDate     *time.Time `json:"due_date"`
	CompletedAt *time.Time `json:"completed_at"`
	CreatedBy   uint      `json:"created_by" gorm:"not null"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Subscription struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	UserID         uint      `json:"user_id" gorm:"not null"`
	User           User      `json:"user,omitempty" gorm:"foreignKey:UserID"`
	Plan           string    `json:"plan" gorm:"not null"` // personal, pro, team
	StripeSubID    string    `json:"stripe_subscription_id" gorm:"uniqueIndex"`
	Status         string    `json:"status" gorm:"default:'active'"` // active, canceled, past_due, unpaid
	CurrentPeriodStart time.Time `json:"current_period_start"`
	CurrentPeriodEnd   time.Time `json:"current_period_end"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type StripeEvent struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	EventID     string    `json:"event_id" gorm:"uniqueIndex;not null"`
	Type        string    `json:"type" gorm:"not null"`
	Processed   bool      `json:"processed" gorm:"default:false"`
	Data        json.RawMessage `json:"data"`
	CreatedAt   time.Time `json:"created_at"`
}

// JWT Claims
type Claims struct {
	UserID   uint   `json:"user_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	WorkspaceID *uint `json:"workspace_id,omitempty"`
}

// Request/Response DTOs
type RegisterRequest struct {
	Email            string `json:"email"`
	Password         string `json:"password"`
	FirstName        string `json:"first_name"`
	LastName         string `json:"last_name"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	User         User   `json:"user"`
}

type CreateWorkspaceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type InviteMemberRequest struct {
	Email    string `json:"email"`
	Role     string `json:"role"`
}

type CreateTaskRequest struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Priority    string  `json:"priority"`
	DueDate     *string `json:"due_date"`
	AssigneeID  *uint   `json:"assignee_id"`
}

type UpdateTaskRequest struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Status      string  `json:"status"`
	Priority    string  `json:"priority"`
	DueDate     *string `json:"due_date"`
	AssigneeID  *uint   `json:"assignee_id"`
}

type CheckoutSessionRequest struct {
	PlanID string `json:"plan_id"` // personal, pro, team
}

// Global variables
var (
	db             *gorm.DB
	jwtSecret      []byte
	upgrader       = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	wsHub         = NewHub()
	rateLimitStore = make(map[string][]time.Time)
)

const (
	RateLimitRequests = 100
	RateLimitWindow   = 60 * time.Second
	JWTExpiry         = 24 * time.Hour
)

func init() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Database connection
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
	)

	var err error
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Auto-migrate tables
	err = db.AutoMigrate(
		&User{},
		&Workspace{},
		&TeamMember{},
		&Task{},
		&Subscription{},
		&StripeEvent{},
	)
	if err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	jwtSecret = []byte(os.Getenv("JWT_SECRET"))
	if len(jwtSecret) == 0 {
		jwtSecret = []byte("development-secret-key-change-in-production")
	}
}

// Middleware
func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next(w, r)
	}
}

func rateLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := strings.Split(r.RemoteAddr, ":")[0]
		now := time.Now()
		
		// Clean old requests
		rateLimitStore[ip] = filterOldRequests(rateLimitStore[ip], now)
		
		// Check limit
		if len(rateLimitStore[ip]) >= RateLimitRequests {
			http.Error(w, `{"error":"Rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		
		// Add current request
		rateLimitStore[ip] = append(rateLimitStore[ip], now)
		next(w, r)
	}
}

func filterOldRequests(requests []time.Time, now time.Time) []time.Time {
	filtered := make([]time.Time, 0)
	for _, t := range requests {
		if now.Sub(t) < RateLimitWindow {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"Authorization header required"}`, http.StatusUnauthorized)
			return
		}

		tokenString := strings.Replace(authHeader, "Bearer ", "", 1)
		claims, err := validateToken(tokenString)
		if err != nil {
			http.Error(w, `{"error":"Invalid token"}`, http.StatusUnauthorized)
			return
		}

		// Add user info to context
		ctx := r.Context()
		ctx = context.WithValue(ctx, "userID", claims.UserID)
		ctx = contextWithValue(ctx, "userEmail", claims.Email)
		ctx = contextWithValue(ctx, "userRole", claims.Role)
		
		next(w, r.WithContext(ctx))
	}
}

func requireWorkspaceAccess(workspaceID uint) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			userID := r.Context().Value("userID").(uint)
			
			var membership TeamMember
			err := db.Where("workspace_id = ? AND user_id = ? AND is_active = ?", 
				workspaceID, userID, true).First(&membership).Error
			if err != nil || membership.Role == "" {
				http.Error(w, `{"error":"Access denied to workspace"}`, http.StatusForbidden)
				return
			}
			
			next(w, r)
		}
	}
}

// Auth handlers
func register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request"}`, http.StatusBadRequest)
		return
	}

	// Validate
	if !strings.Contains(req.Email, "@") || len(req.Password) < 8 {
		http.Error(w, `{"error":"Invalid email or password (min 8 chars)"}`, http.StatusBadRequest)
		return
	}

	// Check if user exists
	var existing User
	if err := db.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		http.Error(w, `{"error":"User already exists"}`, http.StatusConflict)
		return
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, `{"error":"Failed to hash password"}`, http.StatusInternalServerError)
		return
	}

	// Create user
	user := User{
		Email:          req.Email,
		PasswordHash:   string(hash),
		FirstName:      req.FirstName,
		LastName:       req.LastName,
		Role:           "member",
		SubscriptionPlan: "free",
	}

	if err := db.Create(&user).Error; err != nil {
		http.Error(w, `{"error":"Failed to create user"}`, http.StatusInternalServerError)
		return
	}

	// Generate tokens
	token, refreshToken, err := generateTokens(user.ID, user.Email, user.Role)
	if err != nil {
		http.Error(w, `{"error":"Failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(AuthResponse{
		Token:        token,
		RefreshToken: refreshToken,
		User:         sanitizeUser(user),
	})
}

func login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request"}`, http.StatusBadRequest)
		return
	}

	// Find user
	var user User
	if err := db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		http.Error(w, `{"error":"Invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		http.Error(w, `{"error":"Invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	// Generate tokens
	token, refreshToken, err := generateTokens(user.ID, user.Email, user.Role)
	if err != nil {
		http.Error(w, `{"error":"Failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AuthResponse{
		Token:        token,
		RefreshToken: refreshToken,
		User:         sanitizeUser(user),
	})
}

func getProfile(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(uint)
	
	var user User
	if err := db.Preload("Workspaces").First(&user, userID).Error; err != nil {
		http.Error(w, `{"error":"User not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sanitizeUser(user))
}

func refreshToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request"}`, http.StatusBadRequest)
		return
	}

	claims, err := validateRefreshToken(req.RefreshToken)
	if err != nil {
		http.Error(w, `{"error":"Invalid refresh token"}`, http.StatusUnauthorized)
		return
	}

	token, refreshToken, err := generateTokens(claims.UserID, claims.Email, claims.Role)
	if err != nil {
		http.Error(w, `{"error":"Failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token":         token,
		"refresh_token": refreshToken,
	})
}

// Workspace handlers
func createWorkspace(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(uint)
	
	var req CreateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request"}`, http.StatusBadRequest)
		return
	}

	workspace := Workspace{
		Name:        req.Name,
		Description: req.Description,
		OwnerID:     userID,
		PlanTier:    "free",
	}

	if err := db.Create(&workspace).Error; err != nil {
		http.Error(w, `{"error":"Failed to create workspace"}`, http.StatusInternalServerError)
		return
	}

	// Add owner as team member
	teamMember := TeamMember{
		WorkspaceID: workspace.ID,
		UserID:      userID,
		Role:        "owner",
	}
	db.Create(&teamMember)

	// Load associations
	db.Preload("Owner").First(&workspace, workspace.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(workspace)
}

func getWorkspaces(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(uint)
	
	var memberships []TeamMember
	if err := db.Where("user_id = ? AND is_active = ?", userID, true).
		Preload("Workspace").Find(&memberships).Error; err != nil {
		http.Error(w, `{"error":"Failed to fetch workspaces"}`, http.StatusInternalServerError)
		return
	}

	workspaces := make([]Workspace, len(memberships))
	for i, m := range memberships {
		workspaces[i] = m.Workspace
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(workspaces)
}

func inviteMember(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(uint)
	workspaceID := r.URL.Query().Get("workspace_id")
	
	var req InviteMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request"}`, http.StatusBadRequest)
		return
	}

	// Verify user is admin/owner of workspace
	var existingMembership TeamMember
	if err := db.Where("workspace_id = ? AND user_id = ?", workspaceID, userID).
		First(&existingMembership).Error; err != nil || (existingMembership.Role != "owner" && existingMembership.Role != "admin") {
		http.Error(w, `{"error":"Insufficient permissions"}`, http.StatusForbidden)
		return
	}

	// Find invited user
	var invitedUser User
	if err := db.Where("email = ?", req.Email).First(&invitedUser).Error; err != nil {
		http.Error(w, `{"error":"User not found"}`, http.StatusNotFound)
		return
	}

	// Check if already member
	var existing TeamMember
	if err := db.Where("workspace_id = ? AND user_id = ?", workspaceID, invitedUser.ID).
		First(&existing).Error; err == nil {
		http.Error(w, `{"error":"User already a member"}`, http.StatusConflict)
		return
	}

	// Create membership
	membership := TeamMember{
		WorkspaceID: parseUint(workspaceID),
		UserID:      invitedUser.ID,
		Role:        req.Role,
	}
	if err := db.Create(&membership).Error; err != nil {
		http.Error(w, `{"error":"Failed to invite member"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "Member invited successfully"})
}

// Task handlers
func getTasks(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(uint)
	workspaceID := r.URL.Query().Get("workspace_id")
	
	if workspaceID == "" {
		http.Error(w, `{"error":"Workspace ID required"}`, http.StatusBadRequest)
		return
	}

	// Check workspace access
	var membership TeamMember
	if err := db.Where("workspace_id = ? AND user_id = ? AND is_active = ?", 
		workspaceID, userID, true).First(&membership).Error; err != nil {
		http.Error(w, `{"error":"Access denied"}`, http.StatusForbidden)
		return
	}

	var tasks []Task
	query := db.Where("workspace_id = ?", workspaceID)
	
	// Filter by assignee if not admin/owner
	if membership.Role == "member" || membership.Role == "viewer" {
		query = query.Where("assignee_id = ? OR assignee_id IS NULL", userID)
	}
	
	if err := query.Preload("Assignee").Find(&tasks).Error; err != nil {
		http.Error(w, `{"error":"Failed to fetch tasks"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

func createTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(uint)
	workspaceID := r.URL.Query().Get("workspace_id")
	
	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request"}`, http.StatusBadRequest)
		return
	}

	// Verify workspace access
	var membership TeamMember
	if err := db.Where("workspace_id = ? AND user_id = ?", workspaceID, userID).
		First(&membership).Error; err != nil {
		http.Error(w, `{"error":"Access denied"}`, http.StatusForbidden)
		return
	}

	var dueDate *time.Time
	if req.DueDate != nil && *req.DueDate != "" {
		parsed, err := time.Parse(time.RFC3339, *req.DueDate)
		if err == nil {
			dueDate = &parsed
		}
	}

	task := Task{
		WorkspaceID: parseUint(workspaceID),
		Title:       req.Title,
		Description: req.Description,
		Status:      "pending",
		Priority:    req.Priority,
		DueDate:     dueDate,
		AssigneeID:  req.AssigneeID,
		CreatedBy:   userID,
	}

	if err := db.Create(&task).Error; err != nil {
		http.Error(w, `{"error":"Failed to create task"}`, http.StatusInternalServerError)
		return
	}

	// Load associations
	db.Preload("Assignee").First(&task, task.ID)

	// Broadcast via WebSocket
	wsHub.broadcastToWorkspace(workspaceID, "task_created", task)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

func updateTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(uint)
	taskID := r.URL.Query().Get("id")
	
	var req UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request"}`, http.StatusBadRequest)
		return
	}

	// Find task
	var task Task
	if err := db.First(&task, taskID).Error; err != nil {
		http.Error(w, `{"error":"Task not found"}`, http.StatusNotFound)
		return
	}

	// Verify workspace access
	var membership TeamMember
	if err := db.Where("workspace_id = ? AND user_id = ?", task.WorkspaceID, userID).
		First(&membership).Error; err != nil {
		http.Error(w, `{"error":"Access denied"}`, http.StatusForbidden)
		return
	}

	// Only assignee or creator can update
	canUpdate := membership.Role == "owner" || membership.Role == "admin" ||
		task.CreatedBy == userID || (task.AssigneeID != nil && *task.AssigneeID == userID)
	
	if !canUpdate {
		http.Error(w, `{"error":"Insufficient permissions"}`, http.StatusForbidden)
		return
	}

	// Update fields
	task.Title = req.Title
	task.Description = req.Description
	task.Status = req.Status
	task.Priority = req.Priority
	
	if req.AssigneeID != nil {
		task.AssigneeID = req.AssigneeID
	}
	
	if req.DueDate != nil && *req.DueDate != "" {
		parsed, err := time.Parse(time.RFC3339, *req.DueDate)
		if err == nil {
			task.DueDate = &parsed
		}
	}
	
	if task.Status == "completed" && task.CompletedAt == nil {
		now := time.Now()
		task.CompletedAt = &now
	}

	if err := db.Save(&task).Error; err != nil {
		http.Error(w, `{"error":"Failed to update task"}`, http.StatusInternalServerError)
		return
	}

	db.Preload("Assignee").First(&task, task.ID)

	// Broadcast via WebSocket
	wsHub.broadcastToWorkspace(fmt.Sprintf("%d", task.WorkspaceID), "task_updated", task)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func deleteTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(uint)
	taskID := r.URL.Query().Get("id")
	
	var task Task
	if err := db.First(&task, taskID).Error; err != nil {
		http.Error(w, `{"error":"Task not found"}`, http.StatusNotFound)
		return
	}

	// Verify workspace access
	var membership TeamMember
	if err := db.Where("workspace_id = ? AND user_id = ?", task.WorkspaceID, userID).
		First(&membership).Error; err != nil {
		http.Error(w, `{"error":"Access denied"}`, http.StatusForbidden)
		return
	}

	// Only admins/owners or task creator can delete
	if membership.Role != "owner" && membership.Role != "admin" && task.CreatedBy != userID {
		http.Error(w, `{"error":"Insufficient permissions"}`, http.StatusForbidden)
		return
	}

	if err := db.Delete(&task).Error; err != nil {
		http.Error(w, `{"error":"Failed to delete task"}`, http.StatusInternalServerError)
		return
	}

	// Broadcast via WebSocket
	wsHub.broadcastToWorkspace(fmt.Sprintf("%d", task.WorkspaceID), "task_deleted", map[string]uint{"id": task.ID})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Task deleted successfully"})
}

// Analytics handler
func getAnalytics(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(uint)
	workspaceID := r.URL.Query().Get("workspace_id")
	
	// Verify access
	var membership TeamMember
	if err := db.Where("workspace_id = ? AND user_id = ?", workspaceID, userID).
		First(&membership).Error; err != nil {
		http.Error(w, `{"error":"Access denied"}`, http.StatusForbidden)
		return
	}

	// Only admins/owners can view analytics
	if membership.Role != "owner" && membership.Role != "admin" {
		http.Error(w, `{"error":"Insufficient permissions"}`, http.StatusForbidden)
		return
	}

	// Calculate metrics
	var totalTasks int64
	var completedTasks int64
	var pendingTasks int64
	var overdueTasks int64
	
	db.Model(&Task{}).Where("workspace_id = ?", workspaceID).Count(&totalTasks)
	db.Model(&Task{}).Where("workspace_id = ? AND status = ?", workspaceID, "completed").Count(&completedTasks)
	db.Model(&Task{}).Where("workspace_id = ? AND status = ?", workspaceID, "pending").Count(&pendingTasks)
	
	now := time.Now()
	db.Model(&Task{}).Where("workspace_id = ? AND status != ? AND due_date < ?", 
		workspaceID, "completed", now).Count(&overdueTasks)

	// Get tasks by assignee
	var assigneeStats []struct {
		UserID      uint    `json:"user_id"`
		User        User    `json:"user"`
		Total       int64   `json:"total"`
		Completed   int64   `json:"completed"`
		InProgress  int64   `json:"in_progress"`
	}

	db.Table("tasks").
		Select("tasks.assignee_id as user_id, users.*, "+
			"COUNT(*) as total, "+
			"SUM(CASE WHEN tasks.status = 'completed' THEN 1 ELSE 0 END) as completed, "+
			"SUM(CASE WHEN tasks.status = 'in_progress' THEN 1 ELSE 0 END) as in_progress").
		Joins("LEFT JOIN users ON users.id = tasks.assignee_id").
		Where("tasks.workspace_id = ? AND tasks.assignee_id IS NOT NULL", workspaceID).
		Group("tasks.assignee_id, users.id").
		Scan(&assigneeStats)

	// Get completion trend (last 7 days)
	var trend []struct {
		Date     string `json:"date"`
		Completed int64 `json:"completed"`
		Created   int64 `json:"created"`
	}

	for i := 6; i >= 0; i-- {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		start := now.AddDate(0, 0, -i).Truncate(24 * time.Hour)
		end := start.Add(24 * time.Hour)

		var completed, created int64
		db.Model(&Task{}).Where("workspace_id = ? AND status = ? AND completed_at BETWEEN ? AND ?", 
			workspaceID, "completed", start, end).Count(&completed)
		db.Model(&Task{}).Where("workspace_id = ? AND created_at BETWEEN ? AND ?", 
			workspaceID, start, end).Count(&created)

		trend = append(trend, struct {
			Date     string `json:"date"`
			Completed int64 `json:"completed"`
			Created   int64 `json:"created"`
		}{
			Date:     date,
			Completed: completed,
			Created:   created,
		})
	}

	analytics := map[string]interface{}{
		"summary": map[string]interface{}{
			"total":     totalTasks,
			"completed": completedTasks,
			"pending":   pendingTasks,
			"overdue":   overdueTasks,
			"completion_rate": calculateRate(completedTasks, totalTasks),
		},
		"by_assignee":   assigneeStats,
		"trend":         trend,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analytics)
}

func calculateRate(completed, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(completed) / float64(total) * 100
}

// Stripe handlers
func createCheckoutSession(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(uint)
	
	var req CheckoutSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request"}`, http.StatusBadRequest)
		return
	}

	// Get user
	var user User
	if err := db.First(&user, userID).Error; err != nil {
		http.Error(w, `{"error":"User not found"}`, http.StatusNotFound)
		return
	}

	// Plan prices (in cents)
	planPrices := map[string]int{
		"personal": 400,   // $4/month
		"pro":      900,   // $9/month
		"team":     1500,  // $15/month per user
	}

	price, ok := planPrices[req.PlanID]
	if !ok {
		http.Error(w, `{"error":"Invalid plan"}`, http.StatusBadRequest)
		return
	}

	// Create or get Stripe customer
	customerID := user.CustomerID
	if customerID == "" {
		// Create customer in Stripe
		customer, err := stripeClient.Customers.Create(&stripe.CustomerCreateParams{
			Email: &user.Email,
			Name:  stripe.String(fmt.Sprintf("%s %s", user.FirstName, user.LastName)),
		})
		if err != nil {
			http.Error(w, `{"error":"Failed to create Stripe customer"}`, http.StatusInternalServerError)
			return
		}
		customerID = customer.ID
		db.Model(&user).Update("customer_id", customerID)
	}

	// Create checkout session
	checkoutParams := &stripe.CheckoutSessionCreateParams{
		Customer:    &customerID,
		PaymentMethodTypes: stripe.StringSlice([]string{stripe.CheckoutSessionCreateParamsPaymentMethodTypesCard}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency:    stripe.String("usd"),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String(fmt.Sprintf("TodoPro %s Plan", strings.Title(req.PlanID))),
					},
					UnitAmount: stripe.Int64(int64(price)),
				},
				Quantity: stripe.Int64(1),
			},
		},
		Mode:              stripe.String("subscription"),
		SuccessURL:        stripe.String(fmt.Sprintf("%s/success?session_id={CHECKOUT_SESSION_ID}", os.Getenv("APP_URL"))),
		CancelURL:         stripe.String(fmt.Sprintf("%s/pricing", os.Getenv("APP_URL"))),
		AllowPromotionCodes: stripe.Bool(true),
	}

	session, err := stripeClient.Checkout.Sessions.Create(checkoutParams)
	if err != nil {
		http.Error(w, `{"error":"Failed to create checkout session"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"session_id": session.ID,
		"url":        session.URL,
	})
}

func stripeWebhook(w http.ResponseWriter, r *http.Request) {
	const webhookSecret = "whsec_..."
	
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"Failed to read body"}`, http.StatusBadRequest)
		return
	}

	event, err := webhook.ConstructEvent(payload, r.Header.Get("Stripe-Signature"), webhookSecret)
	if err != nil {
		http.Error(w, `{"error":"Invalid signature"}`, http.StatusBadRequest)
		return
	}

	// Store event
	eventData, _ := json.Marshal(event.Data.Object)
	stripeEvent := StripeEvent{
		EventID:  event.ID,
		Type:     event.Type,
		Data:     eventData,
	}
	db.Create(&stripeEvent)

	// Process event
	switch event.Type {
	case "checkout.session.completed":
		handleCheckoutSessionCompleted(event.Data.Object)
	case "customer.subscription.updated":
		handleSubscriptionUpdated(event.Data.Object)
	case "customer.subscription.deleted":
		handleSubscriptionDeleted(event.Data.Object)
	}

	// Mark as processed
	db.Model(&stripeEvent).Update("processed", true)

	w.WriteHeader(http.StatusOK)
}

func handleCheckoutSessionCompleted(session stripe.CheckoutSession) {
	customerID := session.Customer.ID
	subscriptionID := session.Subscription.ID
	
	var user User
	db.Where("stripe_customer_id = ?", customerID).First(&user)
	
	if user.ID != 0 {
		user.SubscriptionPlan = extractPlanFromMetadata(session.Metadata)
		db.Save(&user)
		
		// Create subscription record
		sub := Subscription{
			UserID:            user.ID,
			Plan:              user.SubscriptionPlan,
			StripeSubID:       subscriptionID,
			Status:            "active",
			CurrentPeriodStart: time.Now(),
			CurrentPeriodEnd:  time.Now().Add(30 * 24 * time.Hour),
		}
		db.Create(&sub)
	}
}

func handleSubscriptionUpdated(sub stripe.Subscription) {
	var subscription Subscription
	if err := db.Where("stripe_subscription_id = ?", sub.ID).First(&subscription).Error; err != nil {
		return
	}

	subscription.Status = string(sub.Status)
	subscription.CurrentPeriodStart = time.Unix(sub.CurrentPeriodStart, 0)
	subscription.CurrentPeriodEnd = time.Unix(sub.CurrentPeriodEnd, 0)
	db.Save(&subscription)

	// Update user plan
	var user User
	if err := db.First(&user, subscription.UserID).Error; err == nil {
		user.SubscriptionPlan = extractPlanFromMetadata(sub.Metadata)
		db.Save(&user)
	}
}

func handleSubscriptionDeleted(sub stripe.Subscription) {
	var subscription Subscription
	if err := db.Where("stripe_subscription_id = ?", sub.ID).First(&subscription).Error; err != nil {
		return
	}

	subscription.Status = "canceled"
	db.Save(&subscription)

	var user User
	if err := db.First(&user, subscription.UserID).Error; err == nil {
		user.SubscriptionPlan = "free"
		db.Save(&user)
	}
}

func extractPlanFromMetadata(metadata map[string]string) string {
	if plan, ok := metadata["plan"]; ok {
		return plan
	}
	return "free"
}

// Utility functions
func sanitizeUser(user User) User {
	user.PasswordHash = ""
	return user
}

func parseUint(s string) uint {
	val, _ := strconv.ParseUint(s, 10, 64)
	return uint(val)
}

func main() {
	port := ":5000"
	
	// Routes
	http.HandleFunc("/api/register", rateLimit(withCORS(register)))
	http.HandleFunc("/api/login", rateLimit(withCORS(login)))
	http.HandleFunc("/api/profile", rateLimit(withCORS(requireAuth(getProfile))))
	http.HandleFunc("/api/refresh", rateLimit(withCORS(refreshToken)))
	
	http.HandleFunc("/api/workspaces", rateLimit(withCORS(requireAuth(getWorkspaces))))
	http.HandleFunc("/api/workspaces/create", rateLimit(withCORS(requireAuth(createWorkspace))))
	http.HandleFunc("/api/workspaces/invite", rateLimit(withCORS(requireAuth(inviteMember))))
	
	http.HandleFunc("/api/tasks", rateLimit(withCORS(requireAuth(getTasks))))
	http.HandleFunc("/api/tasks/create", rateLimit(withCORS(requireAuth(createTask))))
	http.HandleFunc("/api/tasks/update", rateLimit(withCORS(requireAuth(updateTask))))
	http.HandleFunc("/api/tasks/delete", rateLimit(withCORS(requireAuth(deleteTask))))
	
	http.HandleFunc("/api/analytics", rateLimit(withCORS(requireAuth(getAnalytics))))
	
	http.HandleFunc("/api/checkout/create", rateLimit(withCORS(requireAuth(createCheckoutSession))))
	http.HandleFunc("/api/stripe/webhook", withCORS(stripeWebhook))
	
	// WebSocket endpoint
	http.HandleFunc("/ws", withCORS(requireAuth(handleWebSocket)))

	http.Handle("/", http.FileServer(http.FS(staticFiles)))
	http.Handle("/style.css", http.FileServer(http.FS(staticFiles)))
	http.Handle("/script.js", http.FileServer(http.FS(staticFiles)))

	fmt.Printf("\n════════════════════════════════════════════════════════════\n")
	fmt.Printf("  TodoPro SaaS API - Production Ready\n")
	fmt.Printf("════════════════════════════════════════════════════════════\n\n")
	fmt.Printf("📍 Running on http://localhost%s\n\n", port)
	fmt.Printf("🔌 API Endpoints:\n")
	fmt.Printf("   POST   /api/register         → User registration\n")
	fmt.Printf("   POST   /api/login            → User login\n")
	fmt.Printf("   GET    /api/profile          → Get current user\n")
	fmt.Printf("   POST   /api/refresh          → Refresh token\n")
	fmt.Printf("   GET    /api/workspaces       → List workspaces\n")
	fmt.Printf("   POST   /api/workspaces/create→ Create workspace\n")
	fmt.Printf("   POST   /api/workspaces/invite→ Invite member\n")
	fmt.Printf("   GET    /api/tasks            → List tasks\n")
	fmt.Printf("   POST   /api/tasks/create     → Create task\n")
	fmt.Printf("   PUT    /api/tasks/update     → Update task\n")
	fmt.Printf("   DELETE /api/tasks/delete     → Delete task\n")
	fmt.Printf("   GET    /api/analytics        → Workspace analytics\n")
	fmt.Printf("   POST   /api/checkout/create  → Create Stripe session\n")
	fmt.Printf("   POST   /api/stripe/webhook   → Stripe webhook\n")
	fmt.Printf("   WS     /ws                   → WebSocket for real-time\n\n")
	fmt.Printf("🔐 PostgreSQL database connected\n")
	fmt.Printf("🔒 JWT authentication enabled\n")
	fmt.Printf("⚡ Rate limiting: %d requests per minute\n", RateLimitRequests)
	fmt.Printf("📊 Real-time updates via WebSocket\n")
	fmt.Printf("💳 Stripe subscription integration\n")
	fmt.Printf("════════════════════════════════════════════════════════════\n\n")

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
