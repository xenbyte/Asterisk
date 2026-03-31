package telegram

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/xenbyte/Asterisk/internal/storage"
)

func (h *Handler) handleCallback(cq *tgbotapi.CallbackQuery) {
	callback := tgbotapi.NewCallback(cq.ID, "")
	if _, err := h.bot.Request(callback); err != nil {
		h.logger.Error("failed to answer callback", "error", err)
	}

	if cq.Message == nil {
		return
	}

	chatID := cq.Message.Chat.ID
	msgID := cq.Message.MessageID
	key := storage.CallbackKey(chatID, msgID)

	resp, err := h.storage.GetCallback(key)
	if err != nil {
		h.logger.Error("failed to get callback data", "error", err, "key", key)
		h.reply(chatID, callbackExpiredReply)
		return
	}
	if resp == nil {
		h.reply(chatID, callbackExpiredReply)
		return
	}

	var text string
	switch cq.Data {
	case "vocab":
		text = formatVocabSection(resp.Vocabulary)
	case "quotes":
		text = formatQuotesSection(resp.Quotes)
	case "missed":
		text = formatMissedSection(resp.Missed)
	case "connections":
		text = formatConnectionsSection(resp.Connections)
	default:
		h.logger.Warn("unknown callback data", "data", cq.Data)
		return
	}

	reply := tgbotapi.NewMessage(chatID, text)
	reply.ParseMode = tgbotapi.ModeHTML
	reply.ReplyToMessageID = msgID
	if _, err := h.bot.Send(reply); err != nil {
		h.logger.Error("failed to send callback reply", "error", err, "section", cq.Data)
	}
}
