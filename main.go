package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
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

type User struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	Email            string    `json:"email" gorm:"uniqueIndex;not null"`
	PasswordHash     string    `json:"-" gorm:"not null"`
	FirstName        string    `json:"first_name"`
	LastName         string    `json:"last_name"`
	Role             string    `json:"role" gorm:"default:'member'"`
	SubscriptionPlan string    `json:"subscription_plan" gorm:"default:'free'"`
	IsActive         bool      `json:"is_active" gorm:"default:true"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Workspaces       []Workspace `json:"workspaces,omitempty" gorm:"foreignKey:OwnerID"`
	Tasks            []Task     `json:"tasks,omitempty" gorm:"foreignKey:AssigneeID"`
}

type Payment struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"not null"`
	User      User      `json:"user,omitempty" gorm:"foreignKey:UserID"`
	Amount    int       `json:"amount" gorm:"not null"`
	TrxID     string    `json:"trx_id" gorm:"uniqueIndex;not null"`
	Status    string    `json:"status" gorm:"default:'pending'"`
	Method    string    `json:"method" gorm:"default:'bkash'"`
	CreatedAt time.Time `json:"created_at"`
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
	Role        string    `json:"role" gorm:"default:'member'"`
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
	Status      string    `json:"status" gorm:"default:'pending'"`
	Priority    string    `json:"priority" gorm:"default:'medium'"`
	AssigneeID  *uint     `json:"assignee_id"`
	Assignee    *User     `json:"assignee,omitempty" gorm:"foreignKey:AssigneeID"`
	DueDate     *time.Time `json:"due_date"`
	CompletedAt *time.Time `json:"completed_at"`
	CreatedBy   uint      `json:"created_by" gorm:"not null"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Claims struct {
	UserID   uint   `json:"user_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	WorkspaceID *uint `json:"workspace_id,omitempty"`
}

type RegisterRequest struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
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

type PaymentSubmitRequest struct {
	TrxID   string `json:"trx_id"`
	Method  string `json:"method"`
}

var (
	db             *gorm.DB
	jwtSecret      []byte
	upgrader       = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	wsHub          = NewHub()
	rateLimitStore = make(map[string][]time.Time)
)

const (
	RateLimitRequests = 100
	RateLimitWindow   = 60 * time.Second
	JWTExpiry         = 24 * time.Hour
)

func contextWithValue(ctx context.Context, key, val string) context.Context {
	return context.WithValue(ctx, key, val)
}

func init() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME"))
	}

	var err error
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Println("PostgreSQL not available (running in demo mode with in-memory store):", err)
		// Continue without database - handlers will return errors gracefully
	} else {
		db.AutoMigrate(&User{}, &Workspace{}, &TeamMember{}, &Task{}, &Payment{})
	}

	jwtSecret = []byte(os.Getenv("JWT_SECRET"))
	if len(jwtSecret) == 0 {
		jwtSecret = []byte("dev-secret-key-change-in-production")
	}
}

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
		rateLimitStore[ip] = filterOldRequests(rateLimitStore[ip], now)
		if len(rateLimitStore[ip]) >= RateLimitRequests {
			http.Error(w, `{"error":"Rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
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
			http.Error(w, `{"error":"Authorization required"}`, http.StatusUnauthorized)
			return
		}
		tokenString := strings.Replace(authHeader, "Bearer ", "", 1)
		claims, err := validateToken(tokenString)
		if err != nil {
			http.Error(w, `{"error":"Invalid token"}`, http.StatusUnauthorized)
			return
		}
		ctx := r.Context()
		ctx = context.WithValue(ctx, "userID", claims.UserID)
		next(w, r.WithContext(ctx))
	}
}

func generateTokens(userID uint, email, role string) (string, string, error) {
	claims := Claims{UserID: userID, Email: email, Role: role}
	token := jwtSign(claims, JWTExpiry)
	refreshToken := jwtSign(claims, JWTExpiry*7)
	return token, refreshToken, nil
}

func jwtSign(claims Claims, expiry time.Duration) string {
	return fmt.Sprintf("%d:%s:%s:%d", claims.UserID, claims.Email, claims.Role, time.Now().Add(expiry).Unix())
}

func validateToken(token string) (*Claims, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid token")
	}
	userID, _ := strconv.ParseUint(parts[0], 10, 64)
	return &Claims{UserID: uint(userID), Email: parts[1], Role: parts[2]}, nil
}

func hashPassword(password string) string {
	h, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(h)
}

func register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request"}`, http.StatusBadRequest)
		return
	}
	if !strings.Contains(req.Email, "@") || len(req.Password) < 8 {
		http.Error(w, `{"error":"Invalid email or password (min 8 chars)"}`, http.StatusBadRequest)
		return
	}
	var existing User
	if err := db.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		http.Error(w, `{"error":"User already exists"}`, http.StatusConflict)
		return
	}
	user := User{
		Email:             req.Email,
		PasswordHash:      hashPassword(req.Password),
		FirstName:         req.FirstName,
		LastName:          req.LastName,
		SubscriptionPlan:  "free",
	}
	if err := db.Create(&user).Error; err != nil {
		http.Error(w, `{"error":"Failed to create user"}`, http.StatusInternalServerError)
		return
	}
	token, refreshToken, _ := generateTokens(user.ID, user.Email, user.Role)
	json.NewEncoder(w).Encode(AuthResponse{Token: token, RefreshToken: refreshToken, User: sanitizeUser(user)})
}

func login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request"}`, http.StatusBadRequest)
		return
	}
	var user User
	if err := db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		http.Error(w, `{"error":"Invalid credentials"}`, http.StatusUnauthorized)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		http.Error(w, `{"error":"Invalid credentials"}`, http.StatusUnauthorized)
		return
	}
	token, refreshToken, _ := generateTokens(user.ID, user.Email, user.Role)
	json.NewEncoder(w).Encode(AuthResponse{Token: token, RefreshToken: refreshToken, User: sanitizeUser(user)})
}

func getProfile(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(uint)
	var user User
	db.Preload("Workspaces").First(&user, userID)
	json.NewEncoder(w).Encode(sanitizeUser(user))
}

func submitPayment(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(uint)
	var req PaymentSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request"}`, http.StatusBadRequest)
		return
	}
	if req.TrxID == "" {
		http.Error(w, `{"error":"Transaction ID required"}`, http.StatusBadRequest)
		return
	}
	if req.Method == "" {
		req.Method = "bkash"
	}
	payment := Payment{UserID: userID, Amount: 500, TrxID: strings.ToUpper(req.TrxID), Status: "pending", Method: req.Method}
	if err := db.Create(&payment).Error; err != nil {
		http.Error(w, `{"error":"Failed to submit payment"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "Payment submitted. Waiting for approval.", "status": "pending"})
}

func approvePayment(w http.ResponseWriter, r *http.Request) {
	trxID := r.URL.Query().Get("trx_id")
	secret := r.URL.Query().Get("secret")
	adminSecret := os.Getenv("ADMIN_SECRET")
	if adminSecret == "" {
		adminSecret = "default-admin-secret-change-me"
	}
	if secret != adminSecret {
		http.Error(w, `{"error":"Invalid admin secret"}`, http.StatusUnauthorized)
		return
	}
	if trxID == "" {
		http.Error(w, `{"error":"Transaction ID required"}`, http.StatusBadRequest)
		return
	}
	var payment Payment
	if err := db.Where("trx_id = ?", strings.ToUpper(trxID)).First(&payment).Error; err != nil {
		http.Error(w, `{"error":"Payment not found"}`, http.StatusNotFound)
		return
	}
	payment.Status = "approved"
	db.Save(&payment)
	db.Model(&User{}).Where("id = ?", payment.UserID).Update("subscription_plan", "pro")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Payment approved. User upgraded to Pro.",
		"user_id": payment.UserID,
		"status":  "approved",
	})
}

func sanitizeUser(user User) User {
	user.PasswordHash = ""
	return user
}

func getWorkspaces(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(uint)
	var memberships []TeamMember
	db.Where("user_id = ? AND is_active = ?", userID, true).Preload("Workspace").Find(&memberships)
	workspaces := make([]Workspace, len(memberships))
	for i, m := range memberships {
		workspaces[i] = m.Workspace
	}
	json.NewEncoder(w).Encode(workspaces)
}

func createWorkspace(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(uint)
	var req CreateWorkspaceRequest
	json.NewDecoder(r.Body).Decode(&req)
	workspace := Workspace{Name: req.Name, Description: req.Description, OwnerID: userID, PlanTier: "free"}
	db.Create(&workspace)
	db.Create(&TeamMember{WorkspaceID: workspace.ID, UserID: userID, Role: "owner"})
	json.NewEncoder(w).Encode(workspace)
}

func getTasks(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspace_id")
	var tasks []Task
	db.Where("workspace_id = ?", workspaceID).Preload("Assignee").Find(&tasks)
	json.NewEncoder(w).Encode(tasks)
}

func createTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(uint)
	workspaceID := r.URL.Query().Get("workspace_id")
	var req CreateTaskRequest
	json.NewDecoder(r.Body).Decode(&req)
	task := Task{
		WorkspaceID: parseUint(workspaceID), Title: req.Title, Description: req.Description,
		Priority: req.Priority, AssigneeID: req.AssigneeID, CreatedBy: userID,
	}
	db.Create(&task)
	wsHub.broadcastToWorkspace(workspaceID, "task_created", task)
	json.NewEncoder(w).Encode(task)
}

func updateTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("id")
	var req UpdateTaskRequest
	json.NewDecoder(r.Body).Decode(&req)
	var task Task
	db.First(&task, taskID)
	task.Title, task.Description, task.Status, task.Priority = req.Title, req.Description, req.Status, req.Priority
	db.Save(&task)
	wsHub.broadcastToWorkspace(fmt.Sprintf("%d", task.WorkspaceID), "task_updated", task)
	json.NewEncoder(w).Encode(task)
}

func deleteTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("id")
	var task Task
	db.First(&task, taskID)
	db.Delete(&task)
	wsHub.broadcastToWorkspace(fmt.Sprintf("%d", task.WorkspaceID), "task_deleted", map[string]uint{"id": task.ID})
	json.NewEncoder(w).Encode(map[string]string{"message": "Deleted"})
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	upgrader.Upgrade(w, r, nil)
}

func parseUint(s string) uint {
	v, _ := strconv.ParseUint(s, 10, 64)
	return uint(v)
}

type Hub struct{ clients map[string]chan interface{} }

func NewHub() *Hub { return &Hub{clients: make(map[string]chan interface{})} }

func (h *Hub) broadcastToWorkspace(workspaceID string, msgType string, payload interface{}) {
	for id, ch := range h.clients {
		if strings.HasPrefix(id, workspaceID) {
			ch <- map[string]interface{}{"type": msgType, "payload": payload}
		}
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}
	http.HandleFunc("/api/register", rateLimit(withCORS(register)))
	http.HandleFunc("/api/login", rateLimit(withCORS(login)))
	http.HandleFunc("/api/profile", rateLimit(withCORS(requireAuth(getProfile))))
	http.HandleFunc("/api/payments/submit", rateLimit(withCORS(requireAuth(submitPayment))))
	http.HandleFunc("/api/admin/approve", withCORS(approvePayment))
	http.HandleFunc("/api/workspaces", rateLimit(withCORS(requireAuth(getWorkspaces))))
	http.HandleFunc("/api/workspaces/create", rateLimit(withCORS(requireAuth(createWorkspace))))
	http.HandleFunc("/api/tasks", rateLimit(withCORS(requireAuth(getTasks))))
	http.HandleFunc("/api/tasks/create", rateLimit(withCORS(requireAuth(createTask))))
	http.HandleFunc("/api/tasks/update", rateLimit(withCORS(requireAuth(updateTask))))
	http.HandleFunc("/api/tasks/delete", rateLimit(withCORS(requireAuth(deleteTask))))
	http.HandleFunc("/ws", withCORS(requireAuth(handleWebSocket)))
	http.Handle("/", http.FileServer(http.FS(staticFiles)))
	fmt.Printf("Server running on :%s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Println("Server error:", err)
	}
}