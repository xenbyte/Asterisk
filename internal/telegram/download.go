package telegram

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Claude accepts exactly these four media types for images.
var allowedMediaTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// detectMediaType returns a Claude-compatible media type. Telegram's download
// endpoint often returns application/octet-stream, so we fall back to
// inferring from the file extension, then default to image/jpeg (safe for
// phone camera photos).
func detectMediaType(contentType, filePath string) string {
	ct := strings.SplitN(contentType, ";", 2)[0]
	ct = strings.TrimSpace(ct)
	if allowedMediaTypes[ct] {
		return ct
	}

	lower := strings.ToLower(filePath)
	switch {
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

const maxPhotoBytes = 20 * 1024 * 1024 // 20 MB

func (h *Handler) downloadPhoto(photos []tgbotapi.PhotoSize) (encoded string, mediaType string, err error) {
	if len(photos) == 0 {
		return "", "", fmt.Errorf("no photo sizes available")
	}

	// Telegram sends multiple sizes; last is largest
	largest := photos[len(photos)-1]

	file, err := h.bot.GetFile(tgbotapi.FileConfig{FileID: largest.FileID})
	if err != nil {
		return "", "", fmt.Errorf("getting file info: %w", err)
	}

	url := file.Link(h.bot.Token)

	resp, err := http.Get(url)
	if err != nil {
		return "", "", fmt.Errorf("downloading photo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, maxPhotoBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", "", fmt.Errorf("reading photo data: %w", err)
	}
	if len(data) > maxPhotoBytes {
		return "", "", fmt.Errorf("photo exceeds %d bytes", maxPhotoBytes)
	}

	ct := detectMediaType(resp.Header.Get("Content-Type"), file.FilePath)

	return base64.StdEncoding.EncodeToString(data), ct, nil
}
