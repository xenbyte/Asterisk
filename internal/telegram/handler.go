package telegram

import (
	"context"
	"log/slog"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/xenbyte/Asterisk/internal/claude"
	"github.com/xenbyte/Asterisk/internal/storage"
)

type Handler struct {
	bot        *tgbotapi.BotAPI
	claude     *claude.Client
	storage    *storage.Storage
	logger     *slog.Logger
	mediaGroup *mediaGroupBuffer
}

func NewHandler(bot *tgbotapi.BotAPI, claude *claude.Client, store *storage.Storage, logger *slog.Logger) *Handler {
	return &Handler{
		bot:        bot,
		claude:     claude,
		storage:    store,
		logger:     logger,
		mediaGroup: newMediaGroupBuffer(1500 * time.Millisecond),
	}
}

func (h *Handler) HandleUpdate(update tgbotapi.Update) {
	// /register is always allowed — handle before auth check.
	if update.Message != nil && update.Message.IsCommand() && update.Message.Command() == "register" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		h.handleRegisterCommand(ctx, update.Message)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if !h.isAuthorized(ctx, update) {
		// Send access-denied message when there is a message sender to reply to.
		if update.Message != nil {
			h.sendMessage(update.Message.Chat.ID, "You don't have access yet. Send /register to request access.")
		}
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

func (h *Handler) isAuthorized(ctx context.Context, update tgbotapi.Update) bool {
	var userID int64
	switch {
	case update.CallbackQuery != nil && update.CallbackQuery.From != nil:
		userID = update.CallbackQuery.From.ID
	case update.Message != nil && update.Message.From != nil:
		userID = update.Message.From.ID
	default:
		return false
	}

	approved, err := h.storage.IsUserApproved(ctx, userID)
	if err != nil {
		h.logger.Error("failed to check user authorization", "user_id", userID, "error", err)
		return false
	}
	if !approved {
		h.logger.Warn("unauthorized access attempt", "user_id", userID)
		return false
	}
	return true
}

// sendMessage sends a plain-text message to a chat.
func (h *Handler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send message", "error", err, "chat_id", chatID)
	}
}

// reply is an alias for sendMessage kept for internal use by other handler files.
func (h *Handler) reply(chatID int64, text string) {
	h.sendMessage(chatID, text)
}

func (h *Handler) replyHTML(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send HTML message", "error", err, "chat_id", chatID)
	}
}
