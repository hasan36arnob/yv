package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrInvalidToken = fmt.Errorf("invalid token")
	ErrExpiredToken = fmt.Errorf("token has expired")
)

// Claims represents JWT claims
type Claims struct {
	UserID    uint   `json:"user_id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateAccessToken creates a JWT access token
func generateAccessToken(userID uint, email, role string) (string, error) {
	claims := Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "TodoPro",
			Subject:   fmt.Sprintf("%d", userID),
			ID:        generateRandomToken(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// GenerateRefreshToken creates a refresh token (stored in DB)
func generateRefreshToken(userID uint) (string, error) {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes), nil
}

// GenerateTokens returns both access and refresh tokens
func generateTokens(userID uint, email, role string) (string, string, error) {
	accessToken, err := generateAccessToken(userID, email, role)
	if err != nil {
		return "", "", err
	}

	refreshToken, err := generateRefreshToken(userID)
	if err != nil {
		return "", "", err
	}

	// Store refresh token hash in database (optional, for rotation)
	go func() {
		hashed, _ := bcrypt.GenerateFromPassword([]byte(refreshToken), bcrypt.DefaultCost)
		// Store hashed token in users table or separate refresh_tokens table
		_ = db.Model(&User{}).Where("id = ?", userID).Update("refresh_token_hash", string(hashed))
	}()

	return accessToken, refreshToken, nil
}

// ValidateToken validates the JWT token and returns claims
func validateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil {
		if err == jwt.ErrorTokenExpired {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}

// ValidateRefreshToken validates refresh tokens (checks against DB if implemented)
func validateRefreshToken(tokenString string) (*Claims, error) {
	// In production, validate against stored hash
	// For simplicity, we'll just extract claims without DB check
	// Implement refresh token rotation and storage in separate table
	
	// Parse without verification to get userID
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}
	
	// This is a simplified approach - in production store refresh tokens in DB
	claims := &Claims{}
	// For demo purposes, we'd need proper token extraction
	// Implement proper refresh token validation with database lookup
	
	return claims, nil
}

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword compares hashed password with plain text
func CheckPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

// SanitizeUser removes sensitive fields from user before JSON response
func sanitizeUser(user User) User {
	user.PasswordHash = ""
	user.RefreshTokenHash = ""
	return user
}

// GetUserByID retrieves user by ID
func GetUserByID(db *gorm.DB, userID uint) (*User, error) {
	var user User
	err := db.First(&user, userID).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByEmail retrieves user by email
func GetUserByEmail(db *gorm.DB, email string) (*User, error) {
	var user User
	err := db.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Context helpers for passing user info through handlers
type contextKey string

const (
	ContextKeyUserID   = "userID"
	ContextKeyEmail    = "userEmail"
	ContextKeyRole     = "userRole"
	ContextKeyWorkspace = "workspaceID"
)

func contextWithValue(parent context.Context, key string, value interface{}) context.Context {
	return context.WithValue(parent, contextKey(key), value)
}

func (c *Claims) ToContext(ctx context.Context) context.Context {
	ctx = contextWithValue(ctx, ContextKeyUserID, c.UserID)
	ctx = contextWithValue(ctx, ContextKeyEmail, c.Email)
	ctx = contextWithValue(ctx, ContextKeyRole, c.Role)
	return ctx
}

// Generate random token
func generateRandomToken() string {
	id := uuid.New()
	return id.String()
}
