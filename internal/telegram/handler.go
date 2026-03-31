package telegram

import (
	"log/slog"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/xenbyte/asterisk/internal/claude"
	"github.com/xenbyte/asterisk/internal/storage"
)

type Handler struct {
	bot        *tgbotapi.BotAPI
	claude     *claude.Client
	storage    *storage.DB
	allowedID  int64
	logger     *slog.Logger
	mediaGroup *mediaGroupBuffer
}

func NewHandler(bot *tgbotapi.BotAPI, claude *claude.Client, store *storage.DB, allowedID int64, logger *slog.Logger) *Handler {
	return &Handler{
		bot:        bot,
		claude:     claude,
		storage:    store,
		allowedID:  allowedID,
		logger:     logger,
		mediaGroup: newMediaGroupBuffer(1500 * time.Millisecond),
	}
}

func (h *Handler) HandleUpdate(update tgbotapi.Update) {
	if !h.isAuthorized(update) {
		return
	}

	switch {
	case update.CallbackQuery != nil:
		h.handleCallback(update.CallbackQuery)
	case update.Message == nil:
		return
	case update.Message.IsCommand():
		h.handleCommand(update.Message)
	case len(update.Message.Photo) > 0:
		if update.Message.MediaGroupID != "" {
			h.mediaGroup.add(update.Message, h.handlePhotoGroup)
		} else {
			h.handlePhotoGroup(update.Message.Chat.ID, []*tgbotapi.Message{update.Message})
		}
	}
}

func (h *Handler) isAuthorized(update tgbotapi.Update) bool {
	var userID int64
	switch {
	case update.CallbackQuery != nil && update.CallbackQuery.From != nil:
		userID = update.CallbackQuery.From.ID
	case update.Message != nil && update.Message.From != nil:
		userID = update.Message.From.ID
	default:
		return false
	}

	if userID != h.allowedID {
		h.logger.Warn("unauthorized access attempt", "user_id", userID)
		return false
	}
	return true
}

func (h *Handler) reply(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send message", "error", err, "chat_id", chatID)
	}
}

func (h *Handler) replyHTML(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send HTML message", "error", err, "chat_id", chatID)
	}
}
