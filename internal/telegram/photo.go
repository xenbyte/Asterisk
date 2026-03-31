package telegram

import (
	"context"
	"fmt"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/xenbyte/Asterisk/internal/claude"
	"github.com/xenbyte/Asterisk/internal/storage"
)

func (h *Handler) handlePhotoGroup(chatID int64, msgs []*tgbotapi.Message) {
	book, err := h.storage.GetBook(chatID)
	if err != nil {
		h.logger.Error("failed to get book", "error", err, "chat_id", chatID)
		h.reply(chatID, claudeErrorReply)
		return
	}
	if book == nil {
		h.reply(chatID, noBookForPhotoReply)
		return
	}

	var images []claude.ImageInput
	for _, msg := range msgs {
		encoded, mediaType, err := h.downloadPhoto(msg.Photo)
		if err != nil {
			h.logger.Error("photo download failed", "error", err, "chat_id", chatID)
			h.reply(chatID, imageDownloadFailedReply)
			return
		}
		images = append(images, claude.ImageInput{
			Base64:    encoded,
			MediaType: mediaType,
		})
	}

	previousSummaries, err := h.storage.GetRecentSummaries(chatID, book, 10)
	if err != nil {
		h.logger.Warn("failed to load previous summaries, continuing without context", "error", err)
		previousSummaries = nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	resp, err := h.claude.AnalyzePages(ctx, images, claude.PromptContext{
		BookTitle:         book.Title,
		Author:            book.Author,
		PreviousSummaries: previousSummaries,
	})
	if err != nil {
		h.logger.Error("claude analysis failed", "error", err, "chat_id", chatID)
		h.reply(chatID, claudeErrorReply)
		return
	}

	if resp.ImageQuality == "retry" {
		h.reply(chatID, imageRetryReply)
		return
	}

	if err := h.storage.StoreAnalysis(chatID, book, resp); err != nil {
		h.logger.Error("failed to persist analysis", "error", err, "chat_id", chatID)
	}

	sentMsg, err := h.sendSummaryWithButtons(chatID, resp.Summary)
	if err != nil {
		h.logger.Error("failed to send summary", "error", err, "chat_id", chatID)
		return
	}

	key := storage.CallbackKey(chatID, sentMsg.MessageID)
	if err := h.storage.StoreCallback(key, resp); err != nil {
		h.logger.Error("failed to persist callback data", "error", err, "chat_id", chatID)
	}
}

func (h *Handler) sendSummaryWithButtons(chatID int64, summary string) (tgbotapi.Message, error) {
	msg := tgbotapi.NewMessage(chatID, summary)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📖 Vocab", "vocab"),
			tgbotapi.NewInlineKeyboardButtonData("💬 Quotes", "quotes"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔗 Connections", "connections"),
			tgbotapi.NewInlineKeyboardButtonData("🔍 What I Missed", "missed"),
		),
	)

	sent, err := h.bot.Send(msg)
	if err != nil {
		return tgbotapi.Message{}, fmt.Errorf("sending summary: %w", err)
	}
	return sent, nil
}
