package telegram

import (
	"context"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/xenbyte/Asterisk/internal/storage"
)

func (h *Handler) handleCommand(msg *tgbotapi.Message) {
	switch msg.Command() {
	case "book":
		h.handleBookCommand(msg)
	case "status":
		h.handleStatusCommand(msg)
	case "quotes":
		h.handleQuotesCommand(msg)
	case "help", "start":
		h.reply(msg.Chat.ID, helpReply)
	default:
		h.reply(msg.Chat.ID, "I don't know that command. Try /help.")
	}
}

func (h *Handler) handleRegisterCommand(ctx context.Context, msg *tgbotapi.Message) {
	userID := int64(msg.From.ID)

	existing, err := h.storage.GetUser(ctx, userID)
	if err != nil {
		h.logger.Error("failed to look up user during registration", "user_id", userID, "error", err)
		h.sendMessage(msg.Chat.ID, "Something went wrong. Please try again later.")
		return
	}

	if existing != nil {
		switch existing.Status {
		case "approved":
			h.sendMessage(msg.Chat.ID, "You're already approved and ready to use Asterisk. Send a photo of a book page to get started.")
			return
		case "pending":
			h.sendMessage(msg.Chat.ID, "Your registration is already pending approval. You'll be notified once an admin reviews it.")
			return
		case "denied":
			h.sendMessage(msg.Chat.ID, "Your registration was denied. Contact the admin if you think this is a mistake.")
			return
		}
	}

	username := msg.From.UserName
	firstName := msg.From.FirstName
	err = h.storage.RegisterUser(ctx, storage.User{
		TelegramID: userID,
		Username:   username,
		FirstName:  firstName,
		Status:     "pending",
	})
	if err != nil {
		h.logger.Error("failed to register user", "user_id", userID, "error", err)
		h.sendMessage(msg.Chat.ID, "Registration failed. Please try again later.")
		return
	}
	h.sendMessage(msg.Chat.ID, "✅ Registration request submitted! An admin will review your request. You'll receive a message once approved.")
}

func (h *Handler) handleBookCommand(msg *tgbotapi.Message) {
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		h.reply(msg.Chat.ID, bookParseErrorReply())
		return
	}

	parts := strings.SplitN(args, " - ", 2)
	if len(parts) != 2 {
		parts = strings.SplitN(args, "-", 2)
	}
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		h.reply(msg.Chat.ID, bookParseErrorReply())
		return
	}

	title := strings.TrimSpace(parts[0])
	author := strings.TrimSpace(parts[1])

	if err := h.storage.SetBook(msg.Chat.ID, &storage.BookContext{
		Title:  title,
		Author: author,
	}); err != nil {
		h.logger.Error("failed to save book", "error", err, "chat_id", msg.Chat.ID)
		h.reply(msg.Chat.ID, "Something went wrong saving that. Try again.")
		return
	}

	h.reply(msg.Chat.ID, bookSetReply(title, author))
}

func (h *Handler) handleStatusCommand(msg *tgbotapi.Message) {
	book, err := h.storage.GetBook(msg.Chat.ID)
	if err != nil {
		h.logger.Error("failed to get book", "error", err, "chat_id", msg.Chat.ID)
		h.reply(msg.Chat.ID, "Something went wrong checking that. Try again.")
		return
	}
	if book == nil {
		h.reply(msg.Chat.ID, statusNoBookReply)
		return
	}
	h.reply(msg.Chat.ID, statusReply(book.Title, book.Author))
}

func (h *Handler) handleQuotesCommand(msg *tgbotapi.Message) {
	book, err := h.storage.GetBook(msg.Chat.ID)
	if err != nil {
		h.logger.Error("failed to get book", "error", err, "chat_id", msg.Chat.ID)
		h.reply(msg.Chat.ID, "Something went wrong. Try again.")
		return
	}
	if book == nil {
		h.reply(msg.Chat.ID, statusNoBookReply)
		return
	}

	quotes, err := h.storage.GetAllQuotes(msg.Chat.ID, book)
	if err != nil {
		h.logger.Error("failed to get quotes", "error", err, "chat_id", msg.Chat.ID)
		h.reply(msg.Chat.ID, "Something went wrong pulling up the quotes. Try again.")
		return
	}

	messages := formatAllQuotes(book.Title, quotes)
	for _, text := range messages {
		h.replyHTML(msg.Chat.ID, text)
	}
}
