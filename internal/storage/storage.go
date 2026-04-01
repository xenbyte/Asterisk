package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xenbyte/Asterisk/internal/analysis"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// BookContext is the active book for a chat session.
type BookContext struct {
	Title  string `json:"title"`
	Author string `json:"author"`
}

// AnalysisRecord stores a single page analysis with metadata.
type AnalysisRecord struct {
	Timestamp time.Time          `json:"timestamp"`
	BookTitle string             `json:"book_title"`
	Author    string             `json:"author"`
	Response  *analysis.Response `json:"response"`
}

// AnalysisResponse is an alias kept for compatibility with callback storage.
type AnalysisResponse = analysis.Response

// User represents a Telegram user registered with the bot.
type User struct {
	TelegramID   int64
	Username     string
	FirstName    string
	Status       string     // "pending", "approved", "denied"
	RegisteredAt time.Time
	ApprovedAt   *time.Time
}

// Storage wraps a PostgreSQL connection pool.
type Storage struct {
	db *sql.DB
}

// New opens a PostgreSQL connection and runs migrations.
func New(ctx context.Context, connStr string) (*Storage, error) {
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	s := &Storage{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return s, nil
}

func (s *Storage) migrate(ctx context.Context) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			telegram_id   BIGINT PRIMARY KEY,
			username      TEXT NOT NULL DEFAULT '',
			first_name    TEXT NOT NULL DEFAULT '',
			status        TEXT NOT NULL DEFAULT 'pending',
			registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			approved_at   TIMESTAMPTZ
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			chat_id     BIGINT PRIMARY KEY,
			book_title  TEXT NOT NULL,
			book_author TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS analyses (
			id         BIGSERIAL PRIMARY KEY,
			chat_id    BIGINT NOT NULL,
			book_key   TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			data       JSONB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS analyses_chat_book ON analyses(chat_id, book_key)`,
		`CREATE TABLE IF NOT EXISTS callbacks (
			key  TEXT PRIMARY KEY,
			data JSONB NOT NULL
		)`,
	}

	for _, m := range migrations {
		if _, err := s.db.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("migration failed (%s...): %w", m[:min(40, len(m))], err)
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Close releases the database connection pool.
func (s *Storage) Close() error {
	return s.db.Close()
}

// BookKey returns a normalised key for a book.
func BookKey(title, author string) string {
	return strings.ToLower(strings.TrimSpace(title)) + "::" + strings.ToLower(strings.TrimSpace(author))
}

// CallbackKey returns a storage key for a callback query.
func CallbackKey(chatID int64, messageID int) string {
	return fmt.Sprintf("%d:%d", chatID, messageID)
}

// --- Users ---

// RegisterUser inserts a new user record. If the user already exists, it is a no-op.
func (s *Storage) RegisterUser(ctx context.Context, u User) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (telegram_id, username, first_name, status, registered_at)
		 VALUES ($1, $2, $3, $4, NOW())
		 ON CONFLICT (telegram_id) DO NOTHING`,
		u.TelegramID, u.Username, u.FirstName, u.Status,
	)
	if err != nil {
		return fmt.Errorf("registering user: %w", err)
	}
	return nil
}

// GetUser returns a user by Telegram ID. Returns nil, nil if not found.
func (s *Storage) GetUser(ctx context.Context, telegramID int64) (*User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT telegram_id, username, first_name, status, registered_at, approved_at
		 FROM users WHERE telegram_id = $1`,
		telegramID,
	)

	var u User
	if err := row.Scan(&u.TelegramID, &u.Username, &u.FirstName, &u.Status, &u.RegisteredAt, &u.ApprovedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting user: %w", err)
	}
	return &u, nil
}

// ListUsers returns users filtered by status. Pass "" or "all" to return all users.
func (s *Storage) ListUsers(ctx context.Context, status string) ([]User, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if status == "" || status == "all" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT telegram_id, username, first_name, status, registered_at, approved_at
			 FROM users ORDER BY registered_at DESC`,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT telegram_id, username, first_name, status, registered_at, approved_at
			 FROM users WHERE status = $1 ORDER BY registered_at DESC`,
			status,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.TelegramID, &u.Username, &u.FirstName, &u.Status, &u.RegisteredAt, &u.ApprovedAt); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// UpdateUserStatus updates a user's status and optional approval timestamp.
func (s *Storage) UpdateUserStatus(ctx context.Context, telegramID int64, status string, approvedAt *time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET status = $1, approved_at = $2 WHERE telegram_id = $3`,
		status, approvedAt, telegramID,
	)
	if err != nil {
		return fmt.Errorf("updating user status: %w", err)
	}
	return nil
}

// IsUserApproved returns true if the user exists and has status "approved".
func (s *Storage) IsUserApproved(ctx context.Context, telegramID int64) (bool, error) {
	u, err := s.GetUser(ctx, telegramID)
	if err != nil {
		return false, err
	}
	if u == nil {
		return false, nil
	}
	return u.Status == "approved", nil
}

// --- Sessions ---

// GetBook returns the active book session for a chat. Returns nil, nil if none set.
// Kept as GetBook for backward compatibility with photo.go / commands.go callers.
func (s *Storage) GetBook(chatID int64) (*BookContext, error) {
	return s.GetSession(context.Background(), chatID)
}

// SetBook persists the active book for a chat.
// Kept for backward compatibility with commands.go callers.
func (s *Storage) SetBook(chatID int64, book *BookContext) error {
	return s.SetSession(context.Background(), chatID, *book)
}

// GetSession returns the active session for a chat. Returns nil, nil if none set.
func (s *Storage) GetSession(ctx context.Context, chatID int64) (*BookContext, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT book_title, book_author FROM sessions WHERE chat_id = $1`,
		chatID,
	)
	var b BookContext
	if err := row.Scan(&b.Title, &b.Author); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting session: %w", err)
	}
	return &b, nil
}

// SetSession persists the active book for a chat.
func (s *Storage) SetSession(ctx context.Context, chatID int64, book BookContext) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (chat_id, book_title, book_author)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (chat_id) DO UPDATE SET book_title = EXCLUDED.book_title, book_author = EXCLUDED.book_author`,
		chatID, book.Title, book.Author,
	)
	if err != nil {
		return fmt.Errorf("setting session: %w", err)
	}
	return nil
}

// --- Analyses ---

// GetRecentAnalyses returns the most-recent analyses for a book in chronological order.
func (s *Storage) GetRecentAnalyses(ctx context.Context, chatID int64, bookKey string, limit int) ([]AnalysisRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT data FROM analyses
		 WHERE chat_id = $1 AND book_key = $2
		 ORDER BY created_at DESC
		 LIMIT $3`,
		chatID, bookKey, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("querying analyses: %w", err)
	}
	defer rows.Close()

	var records []AnalysisRecord
	for rows.Next() {
		var raw json.RawMessage
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scanning analysis: %w", err)
		}
		var rec AnalysisRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			continue
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Reverse so the slice is in chronological order (oldest first).
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
	return records, nil
}

// SaveAnalysis persists an AnalysisRecord for the given chat and book.
func (s *Storage) SaveAnalysis(ctx context.Context, chatID int64, bookKey string, record AnalysisRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshaling analysis: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO analyses (chat_id, book_key, created_at, data) VALUES ($1, $2, NOW(), $3)`,
		chatID, bookKey, data,
	)
	if err != nil {
		return fmt.Errorf("saving analysis: %w", err)
	}
	return nil
}

// StoreAnalysis is the legacy interface used by photo.go.
func (s *Storage) StoreAnalysis(chatID int64, book *BookContext, resp *analysis.Response) error {
	record := AnalysisRecord{
		Timestamp: time.Now(),
		BookTitle: book.Title,
		Author:    book.Author,
		Response:  resp,
	}
	return s.SaveAnalysis(context.Background(), chatID, BookKey(book.Title, book.Author), record)
}

// GetRecentSummaries is the legacy interface used by photo.go.
// Returns summaries most-recent first.
func (s *Storage) GetRecentSummaries(chatID int64, book *BookContext, limit int) ([]string, error) {
	records, err := s.GetRecentAnalyses(context.Background(), chatID, BookKey(book.Title, book.Author), limit)
	if err != nil {
		return nil, err
	}

	// GetRecentAnalyses returns chronological order; reverse to most-recent first for the caller.
	summaries := make([]string, 0, len(records))
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Response != nil {
			summaries = append(summaries, records[i].Response.Summary)
		}
	}
	return summaries, nil
}

// GetAllQuotes is the legacy interface used by commands.go.
func (s *Storage) GetAllQuotes(chatID int64, book *BookContext) ([]analysis.QuoteEntry, error) {
	records, err := s.GetRecentAnalyses(context.Background(), chatID, BookKey(book.Title, book.Author), 10000)
	if err != nil {
		return nil, err
	}

	var quotes []analysis.QuoteEntry
	for _, r := range records {
		if r.Response != nil {
			quotes = append(quotes, r.Response.Quotes...)
		}
	}
	return quotes, nil
}

// --- Callbacks ---

// GetCallback retrieves a stored analysis response by key. Returns nil, nil if not found.
func (s *Storage) GetCallback(ctx context.Context, key string) (*AnalysisResponse, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT data FROM callbacks WHERE key = $1`,
		key,
	)
	var raw json.RawMessage
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting callback: %w", err)
	}
	var resp AnalysisResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("unmarshaling callback: %w", err)
	}
	return &resp, nil
}

// SetCallback persists an analysis response under the given key.
func (s *Storage) SetCallback(ctx context.Context, key string, resp AnalysisResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshaling callback: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO callbacks (key, data) VALUES ($1, $2)
		 ON CONFLICT (key) DO UPDATE SET data = EXCLUDED.data`,
		key, data,
	)
	if err != nil {
		return fmt.Errorf("setting callback: %w", err)
	}
	return nil
}

// StoreCallback is the legacy interface used by photo.go.
func (s *Storage) StoreCallback(key string, resp *analysis.Response) error {
	return s.SetCallback(context.Background(), key, *resp)
}

// GetCallbackLegacy is the legacy interface used by callbacks.go (no context).
func (s *Storage) GetCallbackLegacy(key string) (*analysis.Response, error) {
	return s.GetCallback(context.Background(), key)
}
