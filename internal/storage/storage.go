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

// User represents a Telegram user known to the bot.
type User struct {
	TelegramID int64
	Username   string
	FirstName  string
	FullAccess bool
	FirstSeen  time.Time
}

// BookSummary summarises a book a chat has read.
type BookSummary struct {
	BookKey          string
	Title            string
	Author           string
	Count            int
	LastSeenAt       time.Time
	LatestAnalysisID int64
}

// AnalysisMeta is a lightweight summary of a single analysis.
type AnalysisMeta struct {
	ID        int64
	Title     string
	PageRange string
	CreatedAt time.Time
}

// AnalysisDetail holds the full data for a single analysis.
type AnalysisDetail struct {
	ID         int64
	Title      string
	BookKey    string
	BookTitle  string
	BookAuthor string
	CreatedAt  time.Time
	Data       AnalysisRecord
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
			telegram_id BIGINT PRIMARY KEY,
			username    TEXT NOT NULL DEFAULT '',
			first_name  TEXT NOT NULL DEFAULT '',
			full_access BOOLEAN NOT NULL DEFAULT FALSE,
			first_seen  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS rate_limits (
			user_id BIGINT NOT NULL,
			date    DATE NOT NULL,
			count   INT NOT NULL DEFAULT 0,
			PRIMARY KEY (user_id, date)
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
		`ALTER TABLE analyses ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE analyses ADD COLUMN IF NOT EXISTS page_range TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS daily_limit INT NULL`,
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

// EnsureUser upserts a user record on first interaction.
func (s *Storage) EnsureUser(ctx context.Context, telegramID int64, username, firstName string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users(telegram_id, username, first_name) VALUES($1, $2, $3)
		 ON CONFLICT (telegram_id) DO UPDATE SET username=EXCLUDED.username, first_name=EXCLUDED.first_name`,
		telegramID, username, firstName,
	)
	if err != nil {
		return fmt.Errorf("ensuring user: %w", err)
	}
	return nil
}

// IsFullAccess returns true if the user has full (unlimited) access.
func (s *Storage) IsFullAccess(ctx context.Context, telegramID int64) (bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT full_access FROM users WHERE telegram_id=$1`,
		telegramID,
	)
	var fullAccess bool
	if err := row.Scan(&fullAccess); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("checking full access: %w", err)
	}
	return fullAccess, nil
}

// GetDailyCount returns how many analyses the user has done today (UTC).
func (s *Storage) GetDailyCount(ctx context.Context, userID int64) (int, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT count FROM rate_limits WHERE user_id=$1 AND date=CURRENT_DATE`,
		userID,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("getting daily count: %w", err)
	}
	return count, nil
}

// IncrementDailyCount atomically increments (or inserts) the user's count for today.
func (s *Storage) IncrementDailyCount(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO rate_limits(user_id, date, count) VALUES($1, CURRENT_DATE, 1)
		 ON CONFLICT (user_id, date) DO UPDATE SET count = rate_limits.count + 1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("incrementing daily count: %w", err)
	}
	return nil
}

// GrantFullAccess sets full_access=true for a user (upserting the user row if needed).
func (s *Storage) GrantFullAccess(ctx context.Context, telegramID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users(telegram_id, full_access) VALUES($1, true)
		 ON CONFLICT (telegram_id) DO UPDATE SET full_access=true`,
		telegramID,
	)
	if err != nil {
		return fmt.Errorf("granting full access: %w", err)
	}
	return nil
}

// RevokeFullAccess sets full_access=false.
func (s *Storage) RevokeFullAccess(ctx context.Context, telegramID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET full_access=false WHERE telegram_id=$1`,
		telegramID,
	)
	if err != nil {
		return fmt.Errorf("revoking full access: %w", err)
	}
	return nil
}

// GetEffectiveLimit returns the user's daily analysis limit.
// If the user has a custom limit set, that is returned. Otherwise 15.
func (s *Storage) GetEffectiveLimit(ctx context.Context, userID int64) (int, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT daily_limit FROM users WHERE telegram_id=$1`,
		userID,
	)
	var limit sql.NullInt64
	if err := row.Scan(&limit); err != nil {
		if err == sql.ErrNoRows {
			return 15, nil
		}
		return 15, fmt.Errorf("getting effective limit: %w", err)
	}
	if limit.Valid {
		return int(limit.Int64), nil
	}
	return 15, nil
}

// SetUserLimit sets a custom daily limit for a user.
// Pass limit=0 to reset to the default (NULL in the database).
func (s *Storage) SetUserLimit(ctx context.Context, userID int64, limit int) error {
	if limit == 0 {
		_, err := s.db.ExecContext(ctx,
			`UPDATE users SET daily_limit=NULL WHERE telegram_id=$1`,
			userID,
		)
		if err != nil {
			return fmt.Errorf("resetting user limit: %w", err)
		}
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users(telegram_id, daily_limit) VALUES($1, $2)
		 ON CONFLICT (telegram_id) DO UPDATE SET daily_limit=$2`,
		userID, limit,
	)
	if err != nil {
		return fmt.Errorf("setting user limit: %w", err)
	}
	return nil
}

// UserWithCount extends User with today's usage count and optional custom limit.
type UserWithCount struct {
	User
	DailyCount int
	DailyLimit *int // nil = using default (15)
}

// ListUsersWithCount returns all known users joined with today's rate limit count.
func (s *Storage) ListUsersWithCount(ctx context.Context) ([]UserWithCount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT u.telegram_id, u.username, u.first_name, u.full_access, u.first_seen,
		        COALESCE(r.count, 0) as daily_count,
		        u.daily_limit
		 FROM users u
		 LEFT JOIN rate_limits r ON r.user_id = u.telegram_id AND r.date = CURRENT_DATE
		 ORDER BY u.first_seen DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing users with count: %w", err)
	}
	defer rows.Close()

	var users []UserWithCount
	for rows.Next() {
		var u UserWithCount
		var dailyLimit sql.NullInt64
		if err := rows.Scan(&u.TelegramID, &u.Username, &u.FirstName, &u.FullAccess, &u.FirstSeen, &u.DailyCount, &dailyLimit); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		if dailyLimit.Valid {
			v := int(dailyLimit.Int64)
			u.DailyLimit = &v
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// ListUsers returns all known users.
func (s *Storage) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT telegram_id, username, first_name, full_access, first_seen FROM users ORDER BY first_seen DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.TelegramID, &u.Username, &u.FirstName, &u.FullAccess, &u.FirstSeen); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// GetUserDailyCount returns today's count for a user (alias for GetDailyCount).
func (s *Storage) GetUserDailyCount(ctx context.Context, userID int64) (int, error) {
	return s.GetDailyCount(ctx, userID)
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

// SaveAnalysis persists an AnalysisRecord for the given chat, book, passage title, and page range.
func (s *Storage) SaveAnalysis(ctx context.Context, chatID int64, bookKey string, title string, pageRange string, record AnalysisRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshaling analysis: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO analyses (chat_id, book_key, title, page_range, created_at, data) VALUES ($1, $2, $3, $4, NOW(), $5)`,
		chatID, bookKey, title, pageRange, data,
	)
	if err != nil {
		return fmt.Errorf("saving analysis: %w", err)
	}
	return nil
}

// StoreAnalysis is the legacy interface used by photo.go.
func (s *Storage) StoreAnalysis(chatID int64, book *BookContext, title string, resp *analysis.Response) error {
	pageRange := ""
	if resp != nil {
		pageRange = resp.PageRange
	}
	record := AnalysisRecord{
		Timestamp: time.Now(),
		BookTitle: book.Title,
		Author:    book.Author,
		Response:  resp,
	}
	return s.SaveAnalysis(context.Background(), chatID, BookKey(book.Title, book.Author), title, pageRange, record)
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

// GetAllQuotes is the legacy interface — kept for compatibility.
func (s *Storage) GetAllQuotes(chatID int64, book *BookContext) ([]analysis.Quote, error) {
	records, err := s.GetRecentAnalyses(context.Background(), chatID, BookKey(book.Title, book.Author), 10000)
	if err != nil {
		return nil, err
	}
	var quotes []analysis.Quote
	for _, r := range records {
		if r.Response != nil {
			quotes = append(quotes, r.Response.Quotes...)
		}
	}
	return quotes, nil
}

// BookQuotes groups quotes from a single book.
type BookQuotes struct {
	BookTitle  string
	BookAuthor string
	Quotes     []analysis.Quote
}

// GetAllQuotesForChat returns all quotes across every book analysed in this chat,
// grouped by book, most recently analysed book first.
func (s *Storage) GetAllQuotesForChat(ctx context.Context, chatID int64) ([]BookQuotes, error) {
	books, err := s.ListBooksForChat(ctx, chatID)
	if err != nil {
		return nil, err
	}
	var result []BookQuotes
	for _, b := range books {
		records, err := s.GetRecentAnalyses(ctx, chatID, b.BookKey, 10000)
		if err != nil {
			return nil, err
		}
		var bq BookQuotes
		bq.BookTitle = b.Title
		bq.BookAuthor = b.Author
		for _, r := range records {
			if r.Response != nil {
				bq.Quotes = append(bq.Quotes, r.Response.Quotes...)
			}
		}
		if len(bq.Quotes) > 0 {
			result = append(result, bq)
		}
	}
	return result, nil
}

// ListBooksForChat returns all distinct books the chat has analyzed, most recent first.
func (s *Storage) ListBooksForChat(ctx context.Context, chatID int64) ([]BookSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.book_key,
		        COALESCE(s.book_title, a.book_key) as book_title,
		        COALESCE(s.book_author, '') as book_author,
		        COUNT(a.id) as cnt,
		        MAX(a.created_at) as last_seen,
		        MAX(a.id) as latest_id
		 FROM analyses a
		 LEFT JOIN sessions s ON s.chat_id = a.chat_id
		   AND lower(s.book_title || '::' || s.book_author) = a.book_key
		 WHERE a.chat_id = $1
		 GROUP BY a.book_key, s.book_title, s.book_author
		 ORDER BY last_seen DESC`,
		chatID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing books for chat: %w", err)
	}
	defer rows.Close()

	var books []BookSummary
	for rows.Next() {
		var b BookSummary
		if err := rows.Scan(&b.BookKey, &b.Title, &b.Author, &b.Count, &b.LastSeenAt, &b.LatestAnalysisID); err != nil {
			return nil, fmt.Errorf("scanning book summary: %w", err)
		}
		books = append(books, b)
	}
	return books, rows.Err()
}

// ListAnalysesForBook returns analyses for a specific book, most recent first.
func (s *Storage) ListAnalysesForBook(ctx context.Context, chatID int64, bookKey string) ([]AnalysisMeta, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, page_range, created_at FROM analyses
		 WHERE chat_id = $1 AND book_key = $2
		 ORDER BY created_at DESC
		 LIMIT 10`,
		chatID, bookKey,
	)
	if err != nil {
		return nil, fmt.Errorf("listing analyses for book: %w", err)
	}
	defer rows.Close()

	var metas []AnalysisMeta
	for rows.Next() {
		var m AnalysisMeta
		if err := rows.Scan(&m.ID, &m.Title, &m.PageRange, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning analysis meta: %w", err)
		}
		metas = append(metas, m)
	}
	return metas, rows.Err()
}

// GetAnalysisDetail returns full analysis data by ID.
func (s *Storage) GetAnalysisDetail(ctx context.Context, id int64) (*AnalysisDetail, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT a.id, a.title, a.book_key, a.created_at, a.data,
		        COALESCE(s.book_title, a.book_key) as book_title,
		        COALESCE(s.book_author, '') as book_author
		 FROM analyses a
		 LEFT JOIN sessions s ON s.chat_id = a.chat_id
		   AND lower(s.book_title || '::' || s.book_author) = a.book_key
		 WHERE a.id = $1`,
		id,
	)

	var d AnalysisDetail
	var raw json.RawMessage
	if err := row.Scan(&d.ID, &d.Title, &d.BookKey, &d.CreatedAt, &raw, &d.BookTitle, &d.BookAuthor); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting analysis detail: %w", err)
	}
	if err := json.Unmarshal(raw, &d.Data); err != nil {
		return nil, fmt.Errorf("unmarshaling analysis data: %w", err)
	}
	return &d, nil
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
