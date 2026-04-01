package admin

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xenbyte/Asterisk/internal/storage"
)

// Server is the HTTP admin API.
type Server struct {
	storage *storage.Storage
	token   string
	port    string
}

// New creates a new admin Server.
func New(s *storage.Storage, token, port string) *Server {
	return &Server{storage: s, token: token, port: port}
}

// Start registers routes and begins serving on :<port>.
func (srv *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", srv.handleHealth)
	mux.HandleFunc("/users", srv.authMiddleware(srv.handleUsers))
	mux.HandleFunc("/users/", srv.authMiddleware(srv.handleUserAction))

	addr := ":" + srv.port
	log.Printf("admin API listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}

// authMiddleware enforces Bearer token authentication.
func (srv *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		expected := "Bearer " + srv.token
		if auth != expected {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// handleHealth returns a simple liveness response.
func (srv *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// userJSON is the JSON representation of a user returned by the API.
type userJSON struct {
	TelegramID   int64      `json:"telegram_id"`
	Username     string     `json:"username"`
	FirstName    string     `json:"first_name"`
	Status       string     `json:"status"`
	RegisteredAt time.Time  `json:"registered_at"`
	ApprovedAt   *time.Time `json:"approved_at"`
}

func toUserJSON(u storage.User) userJSON {
	return userJSON{
		TelegramID:   u.TelegramID,
		Username:     u.Username,
		FirstName:    u.FirstName,
		Status:       u.Status,
		RegisteredAt: u.RegisteredAt,
		ApprovedAt:   u.ApprovedAt,
	}
}

// handleUsers handles GET /users
func (srv *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := r.URL.Query().Get("status")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	users, err := srv.storage.ListUsers(ctx, status)
	if err != nil {
		log.Printf("admin: list users error: %v", err)
		httpError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	result := make([]userJSON, 0, len(users))
	for _, u := range users {
		result = append(result, toUserJSON(u))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"users": result})
}

// handleUserAction handles POST /users/{id}/approve and POST /users/{id}/deny
func (srv *Server) handleUserAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Path format: /users/{id}/approve  or  /users/{id}/deny
	// Strip leading slash and split.
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	// parts[0] == "users", parts[1] == id, parts[2] == action
	if len(parts) != 3 || parts[0] != "users" {
		httpError(w, "not found", http.StatusNotFound)
		return
	}

	telegramID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		httpError(w, "invalid user id", http.StatusBadRequest)
		return
	}

	action := parts[2]
	if action != "approve" && action != "deny" {
		httpError(w, "not found", http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var (
		newStatus  string
		approvedAt *time.Time
	)
	if action == "approve" {
		newStatus = "approved"
		now := time.Now()
		approvedAt = &now
	} else {
		newStatus = "denied"
	}

	if err := srv.storage.UpdateUserStatus(ctx, telegramID, newStatus, approvedAt); err != nil {
		log.Printf("admin: update user status error: %v", err)
		httpError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"telegram_id": telegramID,
		"status":      newStatus,
	})
}

func httpError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
