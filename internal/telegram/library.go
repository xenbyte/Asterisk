package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/xenbyte/Asterisk/internal/storage"
)

// analysisButtonLabel builds the inline button label for an analysis entry.
// Format: "p.47–49 · Title" when page range is known, "2 Apr · Title" otherwise.
func analysisButtonLabel(m storage.AnalysisMeta) string {
	title := m.Title
	if title == "" {
		title = "—"
	}
	var label string
	if m.PageRange != "" {
		label = fmt.Sprintf("p.%s · %s", m.PageRange, title)
	} else {
		label = fmt.Sprintf("%s · %s", m.CreatedAt.Format("2 Jan"), title)
	}
	if len(label) > 60 {
		label = label[:57] + "..."
	}
	return label
}

// isLibraryCallback returns true if the callback data belongs to the library navigation system.
func isLibraryCallback(data string) bool {
	return data == "lib" ||
		strings.HasPrefix(data, "bs:") ||
		strings.HasPrefix(data, "an:") ||
		strings.HasPrefix(data, "av:") ||
		strings.HasPrefix(data, "aq:") ||
		strings.HasPrefix(data, "ac:") ||
		strings.HasPrefix(data, "am:")
}

// handleLibraryCallback dispatches a library navigation callback.
func (h *Handler) handleLibraryCallback(cq *tgbotapi.CallbackQuery) {
	// Always dismiss the spinner.
	if _, err := h.bot.Request(tgbotapi.NewCallback(cq.ID, "")); err != nil {
		h.logger.Error("failed to answer library callback", "error", err)
	}

	if cq.Message == nil {
		return
	}

	chatID := cq.Message.Chat.ID
	msgID := cq.Message.MessageID
	data := cq.Data

	switch {
	case data == "lib":
		h.editToLibraryView(chatID, msgID)
	case strings.HasPrefix(data, "bs:"):
		idStr := strings.TrimPrefix(data, "bs:")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return
		}
		h.editToBookView(chatID, msgID, id)
	case strings.HasPrefix(data, "an:"):
		idStr := strings.TrimPrefix(data, "an:")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return
		}
		h.editToAnalysisView(chatID, msgID, id)
	case strings.HasPrefix(data, "av:"):
		idStr := strings.TrimPrefix(data, "av:")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return
		}
		h.editToVocabView(chatID, msgID, id)
	case strings.HasPrefix(data, "aq:"):
		idStr := strings.TrimPrefix(data, "aq:")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return
		}
		h.editToQuotesView(chatID, msgID, id)
	case strings.HasPrefix(data, "ac:"):
		idStr := strings.TrimPrefix(data, "ac:")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return
		}
		h.editToConnectionsView(chatID, msgID, id)
	case strings.HasPrefix(data, "am:"):
		idStr := strings.TrimPrefix(data, "am:")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return
		}
		h.editToMissedView(chatID, msgID, id)
	}
}

// sendLibraryView sends a new message with the library view (called by /books command).
func (h *Handler) sendLibraryView(chatID int64, _ int) {
	ctx := context.Background()
	books, err := h.storage.ListBooksForChat(ctx, chatID)
	if err != nil {
		h.logger.Error("library: failed to list books", "error", err, "chat_id", chatID)
		h.reply(chatID, "Something went wrong loading your library.")
		return
	}

	if len(books) == 0 {
		h.reply(chatID, "Nothing here yet. Send a photo of a page to begin.")
		return
	}

	text := fmt.Sprintf("Your library · %d books", len(books))
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, b := range books {
		label := b.Title
		if b.Author != "" {
			label = fmt.Sprintf("%s — %s", b.Title, b.Author)
		}
		// Truncate if needed for button label (Telegram button text limit).
		if len(label) > 60 {
			label = label[:57] + "..."
		}
		cbData := fmt.Sprintf("bs:%d", b.LatestAnalysisID)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, cbData),
		))
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("library: failed to send library view", "error", err)
	}
}

// editToLibraryView edits an existing message to show the library view.
func (h *Handler) editToLibraryView(chatID int64, msgID int) {
	ctx := context.Background()
	books, err := h.storage.ListBooksForChat(ctx, chatID)
	if err != nil {
		h.logger.Error("library: failed to list books", "error", err, "chat_id", chatID)
		return
	}

	if len(books) == 0 {
		edit := tgbotapi.NewEditMessageText(chatID, msgID, "Nothing here yet. Send a photo of a page to begin.")
		h.bot.Send(edit)
		return
	}

	text := fmt.Sprintf("Your library · %d books", len(books))
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, b := range books {
		label := b.Title
		if b.Author != "" {
			label = fmt.Sprintf("%s — %s", b.Title, b.Author)
		}
		if len(label) > 60 {
			label = label[:57] + "..."
		}
		cbData := fmt.Sprintf("bs:%d", b.LatestAnalysisID)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, cbData),
		))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, text, keyboard)
	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("library: failed to edit to library view", "error", err)
	}
}

// editToBookView edits the message to show analyses for the book containing analysisID.
func (h *Handler) editToBookView(chatID int64, msgID int, analysisID int64) {
	ctx := context.Background()
	detail, err := h.storage.GetAnalysisDetail(ctx, analysisID)
	if err != nil || detail == nil {
		h.logger.Error("library: failed to get analysis detail for book view", "error", err, "id", analysisID)
		return
	}

	metas, err := h.storage.ListAnalysesForBook(ctx, chatID, detail.BookKey)
	if err != nil {
		h.logger.Error("library: failed to list analyses for book", "error", err)
		return
	}

	bookLabel := detail.BookTitle
	if detail.BookAuthor != "" {
		bookLabel = fmt.Sprintf("%s — %s", detail.BookTitle, detail.BookAuthor)
	}
	text := fmt.Sprintf("%s\n%d passages", bookLabel, len(metas))

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, m := range metas {
		label := analysisButtonLabel(m)
		cbData := fmt.Sprintf("an:%d", m.ID)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, cbData),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("← Library", "lib"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, text, keyboard)
	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("library: failed to edit to book view", "error", err)
	}
}

// editToAnalysisView edits the message to show the summary + nav buttons for one analysis.
func (h *Handler) editToAnalysisView(chatID int64, msgID int, analysisID int64) {
	ctx := context.Background()
	detail, err := h.storage.GetAnalysisDetail(ctx, analysisID)
	if err != nil || detail == nil {
		h.logger.Error("library: failed to get analysis detail", "error", err, "id", analysisID)
		return
	}

	bookLabel := detail.BookTitle
	if detail.BookAuthor != "" {
		bookLabel = fmt.Sprintf("%s — %s", detail.BookTitle, detail.BookAuthor)
	}

	// Build subheader: "p.47–49 · Eliot · 2 Apr" or "Eliot · 2 Apr" if no page range.
	date := detail.CreatedAt.Format("2 Jan 2006")
	pageRange := ""
	if detail.Data.Response != nil {
		pageRange = detail.Data.Response.PageRange
	}
	var subheader string
	if pageRange != "" {
		subheader = fmt.Sprintf("p.%s · %s · %s", pageRange, bookLabel, date)
	} else {
		subheader = fmt.Sprintf("%s · %s", bookLabel, date)
	}

	summary := ""
	if detail.Data.Response != nil {
		summary = detail.Data.Response.Summary
	}

	text := fmt.Sprintf("%s\n%s\n\n%s", detail.Title, subheader, summary)

	idStr := strconv.FormatInt(analysisID, 10)
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Vocabulary", "av:"+idStr),
			tgbotapi.NewInlineKeyboardButtonData("Quotes", "aq:"+idStr),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Connections", "ac:"+idStr),
			tgbotapi.NewInlineKeyboardButtonData("What I Missed", "am:"+idStr),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("← Back", fmt.Sprintf("bs:%d", analysisID)),
		),
	)

	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, text, keyboard)
	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("library: failed to edit to analysis view", "error", err)
	}
}

// editToVocabView edits the message to show vocabulary for an analysis.
func (h *Handler) editToVocabView(chatID int64, msgID int, analysisID int64) {
	ctx := context.Background()
	detail, err := h.storage.GetAnalysisDetail(ctx, analysisID)
	if err != nil || detail == nil {
		h.logger.Error("library: failed to get analysis detail for vocab", "error", err, "id", analysisID)
		return
	}

	idStr := strconv.FormatInt(analysisID, 10)
	var sb strings.Builder
	title := detail.Title
	if title == "" {
		title = "This passage"
	}
	fmt.Fprintf(&sb, "%s · Vocabulary\n\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500", title)

	if detail.Data.Response == nil || len(detail.Data.Response.Vocabulary) == 0 {
		sb.WriteString("\n\nNo vocabulary noted for this passage.")
	} else {
		for _, v := range detail.Data.Response.Vocabulary {
			fmt.Fprintf(&sb, "\n\n%s\n%s\n\n\u201c%s\u201d", v.Word, v.Definition, v.Sentence)
		}
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("← Analysis", "an:"+idStr),
		),
	)
	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, sb.String(), keyboard)
	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("library: failed to edit to vocab view", "error", err)
	}
}

// editToQuotesView edits the message to show quotes for an analysis.
func (h *Handler) editToQuotesView(chatID int64, msgID int, analysisID int64) {
	ctx := context.Background()
	detail, err := h.storage.GetAnalysisDetail(ctx, analysisID)
	if err != nil || detail == nil {
		h.logger.Error("library: failed to get analysis detail for quotes", "error", err, "id", analysisID)
		return
	}

	idStr := strconv.FormatInt(analysisID, 10)
	var sb strings.Builder
	title := detail.Title
	if title == "" {
		title = "This passage"
	}
	fmt.Fprintf(&sb, "%s · Quotes\n\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500", title)

	if detail.Data.Response == nil || len(detail.Data.Response.Quotes) == 0 {
		sb.WriteString("\n\nNo quotes noted for this passage.")
	} else {
		for _, q := range detail.Data.Response.Quotes {
			fmt.Fprintf(&sb, "\n\n\u201c%s\u201d\n\u2014 %s", q.Text, q.Note)
		}
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("← Analysis", "an:"+idStr),
		),
	)
	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, sb.String(), keyboard)
	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("library: failed to edit to quotes view", "error", err)
	}
}

// editToConnectionsView edits the message to show connections for an analysis.
func (h *Handler) editToConnectionsView(chatID int64, msgID int, analysisID int64) {
	ctx := context.Background()
	detail, err := h.storage.GetAnalysisDetail(ctx, analysisID)
	if err != nil || detail == nil {
		h.logger.Error("library: failed to get analysis detail for connections", "error", err, "id", analysisID)
		return
	}

	idStr := strconv.FormatInt(analysisID, 10)
	var sb strings.Builder
	title := detail.Title
	if title == "" {
		title = "This passage"
	}
	fmt.Fprintf(&sb, "%s · Connections\n\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500", title)

	connections := ""
	if detail.Data.Response != nil {
		connections = detail.Data.Response.Connections
	}
	if connections == "" {
		sb.WriteString("\n\nNo notable connections to earlier passages.")
	} else {
		fmt.Fprintf(&sb, "\n\n%s", connections)
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("← Analysis", "an:"+idStr),
		),
	)
	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, sb.String(), keyboard)
	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("library: failed to edit to connections view", "error", err)
	}
}

// editToMissedView edits the message to show "what I missed" for an analysis.
func (h *Handler) editToMissedView(chatID int64, msgID int, analysisID int64) {
	ctx := context.Background()
	detail, err := h.storage.GetAnalysisDetail(ctx, analysisID)
	if err != nil || detail == nil {
		h.logger.Error("library: failed to get analysis detail for missed", "error", err, "id", analysisID)
		return
	}

	idStr := strconv.FormatInt(analysisID, 10)
	var sb strings.Builder
	title := detail.Title
	if title == "" {
		title = "This passage"
	}
	fmt.Fprintf(&sb, "%s · What I Missed\n\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500", title)

	missed := ""
	if detail.Data.Response != nil {
		missed = detail.Data.Response.Missed
	}
	if missed == "" {
		sb.WriteString("\n\nNothing flagged for this passage.")
	} else {
		fmt.Fprintf(&sb, "\n\n%s", missed)
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("← Analysis", "an:"+idStr),
		),
	)
	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, sb.String(), keyboard)
	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("library: failed to edit to missed view", "error", err)
	}
}
