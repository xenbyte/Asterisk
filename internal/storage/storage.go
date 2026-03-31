package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xenbyte/asterisk/internal/analysis"
	"go.etcd.io/bbolt"
)

var (
	bucketSessions  = []byte("sessions")
	bucketAnalyses  = []byte("analyses")
	bucketCallbacks = []byte("callbacks")
)

type BookContext struct {
	Title  string `json:"title"`
	Author string `json:"author"`
}

type AnalysisRecord struct {
	Timestamp time.Time          `json:"timestamp"`
	BookTitle string             `json:"book_title"`
	Author    string             `json:"author"`
	Response  *analysis.Response `json:"response"`
}

type DB struct {
	bolt *bbolt.DB
}

func Open(path string) (*DB, error) {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		for _, b := range [][]byte{bucketSessions, bucketAnalyses, bucketCallbacks} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("creating buckets: %w", err)
	}

	return &DB{bolt: db}, nil
}

func (db *DB) Close() error {
	return db.bolt.Close()
}

func BookKey(title, author string) string {
	return strings.ToLower(strings.TrimSpace(title)) + "::" + strings.ToLower(strings.TrimSpace(author))
}

func CallbackKey(chatID int64, messageID int) string {
	return fmt.Sprintf("%d:%d", chatID, messageID)
}

// --- Sessions ---

func (db *DB) SetBook(chatID int64, book *BookContext) error {
	return db.bolt.Update(func(tx *bbolt.Tx) error {
		data, err := json.Marshal(book)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketSessions).Put([]byte(fmt.Sprintf("%d", chatID)), data)
	})
}

func (db *DB) GetBook(chatID int64) (*BookContext, error) {
	var book BookContext
	var found bool
	err := db.bolt.View(func(tx *bbolt.Tx) error {
		data := tx.Bucket(bucketSessions).Get([]byte(fmt.Sprintf("%d", chatID)))
		if data == nil {
			return nil
		}
		found = true
		return json.Unmarshal(data, &book)
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &book, nil
}

// --- Analyses ---

func (db *DB) StoreAnalysis(chatID int64, book *BookContext, resp *analysis.Response) error {
	record := AnalysisRecord{
		Timestamp: time.Now(),
		BookTitle: book.Title,
		Author:    book.Author,
		Response:  resp,
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshaling analysis: %w", err)
	}

	key := fmt.Sprintf("%d:%s:%020d", chatID, BookKey(book.Title, book.Author), record.Timestamp.UnixNano())

	return db.bolt.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketAnalyses).Put([]byte(key), data)
	})
}

// GetRecentSummaries returns the most recent summaries for a book, most-recent first.
func (db *DB) GetRecentSummaries(chatID int64, book *BookContext, limit int) ([]string, error) {
	prefix := []byte(fmt.Sprintf("%d:%s:", chatID, BookKey(book.Title, book.Author)))
	var summaries []string

	err := db.bolt.View(func(tx *bbolt.Tx) error {
		c := tx.Bucket(bucketAnalyses).Cursor()

		var all []string
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			var record AnalysisRecord
			if err := json.Unmarshal(v, &record); err != nil {
				continue
			}
			all = append(all, record.Response.Summary)
		}

		if len(all) > limit {
			all = all[len(all)-limit:]
		}
		for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
			all[i], all[j] = all[j], all[i]
		}
		summaries = all
		return nil
	})

	return summaries, err
}

func (db *DB) GetAllQuotes(chatID int64, book *BookContext) ([]analysis.QuoteEntry, error) {
	prefix := []byte(fmt.Sprintf("%d:%s:", chatID, BookKey(book.Title, book.Author)))
	var quotes []analysis.QuoteEntry

	err := db.bolt.View(func(tx *bbolt.Tx) error {
		c := tx.Bucket(bucketAnalyses).Cursor()
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			var record AnalysisRecord
			if err := json.Unmarshal(v, &record); err != nil {
				continue
			}
			quotes = append(quotes, record.Response.Quotes...)
		}
		return nil
	})

	return quotes, err
}

// --- Callbacks (persistent button data) ---

func (db *DB) StoreCallback(key string, resp *analysis.Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshaling callback: %w", err)
	}
	return db.bolt.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketCallbacks).Put([]byte(key), data)
	})
}

func (db *DB) GetCallback(key string) (*analysis.Response, error) {
	var resp analysis.Response
	var found bool
	err := db.bolt.View(func(tx *bbolt.Tx) error {
		data := tx.Bucket(bucketCallbacks).Get([]byte(key))
		if data == nil {
			return nil
		}
		found = true
		return json.Unmarshal(data, &resp)
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &resp, nil
}
