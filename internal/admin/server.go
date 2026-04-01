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
	TelegramID int64     `json:"telegram_id"`
	Username   string    `json:"username"`
	FirstName  string    `json:"first_name"`
	FullAccess bool      `json:"full_access"`
	FirstSeen  time.Time `json:"first_seen"`
	DailyCount int       `json:"daily_count"`
}

// handleUsers handles GET /users
func (srv *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	users, err := srv.storage.ListUsersWithCount(ctx)
	if err != nil {
		log.Printf("admin: list users error: %v", err)
		httpError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	result := make([]userJSON, 0, len(users))
	for _, u := range users {
		result = append(result, userJSON{
			TelegramID: u.TelegramID,
			Username:   u.Username,
			FirstName:  u.FirstName,
			FullAccess: u.FullAccess,
			FirstSeen:  u.FirstSeen,
			DailyCount: u.DailyCount,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"users": result})
}

// handleUserAction handles POST /users/{id}/grant and POST /users/{id}/revoke
func (srv *Server) handleUserAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Path format: /users/{id}/grant  or  /users/{id}/revoke
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
	if action != "grant" && action != "revoke" {
		httpError(w, "not found", http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if action == "grant" {
		if err := srv.storage.GrantFullAccess(ctx, telegramID); err != nil {
			log.Printf("admin: grant full access error: %v", err)
			httpError(w, "internal server error", http.StatusInternalServerError)
			return
		}
	} else {
		if err := srv.storage.RevokeFullAccess(ctx, telegramID); err != nil {
			log.Printf("admin: revoke full access error: %v", err)
			httpError(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"telegram_id": telegramID,
		"action":      action,
	})
}

func httpError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
