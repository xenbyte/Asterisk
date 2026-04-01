package telegram

import (
	"context"
	"fmt"
	"strconv"
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

// isAdminCommand reports whether a message is an /admin command.
func isAdminCommand(msg *tgbotapi.Message) bool {
	if msg == nil || msg.From == nil {
		return false
	}
	return msg.IsCommand() && msg.Command() == "admin"
}

// handleAdminCommand dispatches /admin subcommands for the verified admin user.
func (h *Handler) handleAdminCommand(ctx context.Context, msg *tgbotapi.Message) {
	args := strings.Fields(strings.TrimSpace(msg.CommandArguments()))

	if len(args) == 0 {
		h.reply(msg.Chat.ID, adminUsage)
		return
	}

	switch args[0] {
	case "users":
		h.adminUsers(ctx, msg.Chat.ID)
	case "grant":
		if len(args) < 2 {
			h.reply(msg.Chat.ID, adminUsage)
			return
		}
		h.adminGrant(ctx, msg.Chat.ID, args[1])
	case "revoke":
		if len(args) < 2 {
			h.reply(msg.Chat.ID, adminUsage)
			return
		}
		h.adminRevoke(ctx, msg.Chat.ID, args[1])
	default:
		h.reply(msg.Chat.ID, adminUsage)
	}
}

const adminUsage = "Admin commands:\n/admin users\n/admin grant <telegram_id>\n/admin revoke <telegram_id>"

func (h *Handler) adminUsers(ctx context.Context, chatID int64) {
	users, err := h.storage.ListUsersWithCount(ctx)
	if err != nil {
		h.logger.Error("admin: list users error", "error", err)
		h.reply(chatID, "Something went wrong listing users.")
		return
	}
	if len(users) == 0 {
		h.reply(chatID, "No users yet.")
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "👥 Users (%d)\n", len(users))
	for _, u := range users {
		sb.WriteString("\n")
		if u.Username != "" {
			fmt.Fprintf(&sb, "@%s · %d\n", u.Username, u.TelegramID)
		} else {
			fmt.Fprintf(&sb, "%s · %d\n", u.FirstName, u.TelegramID)
		}
		if u.FullAccess {
			sb.WriteString("📊 — · 👑 full access\n")
		} else {
			fmt.Fprintf(&sb, "📊 %d/15 today · limited\n", u.DailyCount)
		}
	}

	h.reply(chatID, strings.TrimRight(sb.String(), "\n"))
}

func (h *Handler) adminGrant(ctx context.Context, chatID int64, idStr string) {
	telegramID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		h.reply(chatID, "Invalid Telegram ID.")
		return
	}
	if err := h.storage.GrantFullAccess(ctx, telegramID); err != nil {
		h.logger.Error("admin: grant full access error", "error", err)
		h.reply(chatID, "Something went wrong granting access.")
		return
	}
	h.reply(chatID, fmt.Sprintf("✅ Full access granted to %d.", telegramID))
}

func (h *Handler) adminRevoke(ctx context.Context, chatID int64, idStr string) {
	telegramID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		h.reply(chatID, "Invalid Telegram ID.")
		return
	}
	if err := h.storage.RevokeFullAccess(ctx, telegramID); err != nil {
		h.logger.Error("admin: revoke full access error", "error", err)
		h.reply(chatID, "Something went wrong revoking access.")
		return
	}
	h.reply(chatID, fmt.Sprintf("🔒 Full access revoked for %d. Default limit (15/day) applies.", telegramID))
}
