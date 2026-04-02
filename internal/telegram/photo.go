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

	// Trim to the first 4 photos and notify the user if more were sent.
	if len(msgs) > 4 {
		msgs = msgs[:4]
		h.sendMessage(chatID, "Only the first 4 photos will be analysed per message.")
	}

	// Enforce rate limit — derive user ID from the first message.
	var userID int64
	if len(msgs) > 0 && msgs[0].From != nil {
		userID = msgs[0].From.ID
	}

	if userID != 0 {
		ctx := context.Background()
		full, err := h.storage.IsFullAccess(ctx, userID)
		if err != nil {
			h.logger.Error("failed to check full access", "user_id", userID, "error", err)
			// proceed without blocking on error
		} else if !full {
			count, err := h.storage.GetDailyCount(ctx, userID)
			if err != nil {
				h.logger.Error("failed to get daily count", "user_id", userID, "error", err)
				// proceed without blocking on error
			} else {
				limit, err := h.storage.GetEffectiveLimit(ctx, userID)
				if err != nil {
					h.logger.Error("failed to get effective limit", "user_id", userID, "error", err)
					limit = 15
				}
				if count >= limit {
					h.sendMessage(chatID, rateLimitReply)
					return
				}
			}
		}
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

	// Send typing indicator before the Claude call.
	chatAction := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	h.bot.Send(chatAction)

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

	if err := h.storage.StoreAnalysis(chatID, book, resp.Title, resp); err != nil {
		h.logger.Error("failed to persist analysis", "error", err, "chat_id", chatID)
	}

	// Increment daily usage counter after a successful analysis.
	if userID != 0 {
		if err := h.storage.IncrementDailyCount(context.Background(), userID); err != nil {
			h.logger.Error("failed to increment daily count", "user_id", userID, "error", err)
		}
	}

	sentMsg, err := h.sendSummaryWithButtons(chatID, resp.Title, resp.Summary)
	if err != nil {
		h.logger.Error("failed to send summary", "error", err, "chat_id", chatID)
		return
	}

	key := storage.CallbackKey(chatID, sentMsg.MessageID)
	if err := h.storage.StoreCallback(key, resp); err != nil {
		h.logger.Error("failed to persist callback data", "error", err, "chat_id", chatID)
	}
}

func (h *Handler) sendSummaryWithButtons(chatID int64, title, summary string) (tgbotapi.Message, error) {
	text := fmt.Sprintf("<b>%s</b>\n\n%s", escapeHTML(title), escapeHTML(summary))
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Vocab", "vocab"),
			tgbotapi.NewInlineKeyboardButtonData("Quotes", "quotes"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Connections", "connections"),
			tgbotapi.NewInlineKeyboardButtonData("What I Missed", "missed"),
		),
	)

	sent, err := h.bot.Send(msg)
	if err != nil {
		return tgbotapi.Message{}, fmt.Errorf("sending summary: %w", err)
	}
	return sent, nil
}
