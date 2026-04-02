package analysis

import (
	"encoding/json"
	"strings"
)

type VocabEntry struct {
	Word       string `json:"word"`
	Definition string `json:"definition"`
	Sentence   string `json:"sentence"`
}

type Quote struct {
	Text string `json:"text"`
	Note string `json:"note"`
}

type Response struct {
	Title        string       `json:"title"`
	Summary      string       `json:"summary"`
	Vocabulary   []VocabEntry `json:"vocabulary"`
	Quotes       []Quote      `json:"quotes"`
	Missed       string       `json:"missed"`
	Connections  string       `json:"connections"`
	ImageQuality string       `json:"image_quality"`
}

// UnmarshalJSON handles both the current schema (missed/connections as strings)
// and the legacy schema (missed/connections as arrays) stored in the database.
func (r *Response) UnmarshalJSON(data []byte) error {
	// Use a shadow type to avoid infinite recursion.
	type raw struct {
		Title        string          `json:"title"`
		Summary      string          `json:"summary"`
		Vocabulary   []VocabEntry    `json:"vocabulary"`
		Quotes       []Quote         `json:"quotes"`
		Missed       json.RawMessage `json:"missed"`
		Connections  json.RawMessage `json:"connections"`
		ImageQuality string          `json:"image_quality"`
	}
	var v raw
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	r.Title = v.Title
	r.Summary = v.Summary
	r.Vocabulary = v.Vocabulary
	r.Quotes = v.Quotes
	r.ImageQuality = v.ImageQuality
	r.Missed = coerceToString(v.Missed)
	r.Connections = coerceToString(v.Connections)
	return nil
}

// coerceToString converts a raw JSON value to a string regardless of whether
// it was encoded as a JSON string, an array of strings, or null.
func coerceToString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	// Try string first (current format).
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Fall back to array of strings (legacy format).
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return strings.Join(arr, "\n")
	}
	// Last resort: strip quotes from raw value.
	return strings.Trim(string(raw), `"`)
}
