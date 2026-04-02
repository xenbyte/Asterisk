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
	adminID    int64
}

func NewHandler(bot *tgbotapi.BotAPI, claude *claude.Client, store *storage.Storage, logger *slog.Logger, adminID int64) *Handler {
	return &Handler{
		bot:        bot,
		claude:     claude,
		storage:    store,
		logger:     logger,
		mediaGroup: newMediaGroupBuffer(1500 * time.Millisecond),
		adminID:    adminID,
	}
}

func (h *Handler) HandleUpdate(update tgbotapi.Update) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Lazily register the user on every interaction.
	var userID int64
	var username, firstName string
	switch {
	case update.Message != nil && update.Message.From != nil:
		userID = update.Message.From.ID
		username = update.Message.From.UserName
		firstName = update.Message.From.FirstName
	case update.CallbackQuery != nil && update.CallbackQuery.From != nil:
		userID = update.CallbackQuery.From.ID
		username = update.CallbackQuery.From.UserName
		firstName = update.CallbackQuery.From.FirstName
	}
	if userID != 0 {
		if err := h.storage.EnsureUser(ctx, userID, username, firstName); err != nil {
			h.logger.Error("failed to ensure user", "user_id", userID, "error", err)
		}
	}

	switch {
	case update.CallbackQuery != nil:
		if isLibraryCallback(update.CallbackQuery.Data) {
			h.handleLibraryCallback(update.CallbackQuery)
		} else {
			h.handleCallback(update.CallbackQuery)
		}
	case update.Message == nil:
		return
	case isAdminCommand(update.Message):
		if int64(update.Message.From.ID) != h.adminID {
			// silently ignore — don't reveal admin commands exist
			return
		}
		h.handleAdminCommand(ctx, update.Message)
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
