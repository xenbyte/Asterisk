package analysis

import (
	"encoding/json"
	"fmt"
	"strings"
)

func Parse(raw string) (*Response, error) {
	cleaned := stripMarkdownFences(raw)
	var resp Response
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if err := validate(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// stripMarkdownFences removes ```json ... ``` wrappers that Claude
// sometimes adds despite being told not to.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// Remove opening fence (```json or just ```)
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		// Remove closing fence
		if strings.HasSuffix(s, "```") {
			s = s[:len(s)-3]
		}
		s = strings.TrimSpace(s)
	}
	return s
}

func validate(r *Response) error {
	if r.Summary == "" {
		return fmt.Errorf("missing required field: summary")
	}
	if r.ImageQuality != "ok" && r.ImageQuality != "retry" {
		return fmt.Errorf("image_quality must be \"ok\" or \"retry\", got %q", r.ImageQuality)
	}
	if r.ImageQuality == "ok" {
		if r.Vocabulary == nil {
			return fmt.Errorf("missing required field: vocabulary")
		}
		if r.Quotes == nil {
			return fmt.Errorf("missing required field: quotes")
		}
		if r.Missed == nil {
			return fmt.Errorf("missing required field: missed")
		}
	}
	return nil
}
