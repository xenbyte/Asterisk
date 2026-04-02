package telegram

import (
	"fmt"
	"strings"

	"github.com/xenbyte/Asterisk/internal/analysis"
	"github.com/xenbyte/Asterisk/internal/storage"
)

func bookSetReply(title, author string) string {
	return fmt.Sprintf("Reading %s by %s.", title, author)
}

func statusReply(title, author string) string {
	return fmt.Sprintf("Currently reading: %s by %s.", title, author)
}

const statusNoBookReply = "No book set. Use /book Title — Author."

const rateLimitReply = "You've read your way through today's quota. Come back tomorrow."

const helpReply = `Asterisk — a reading companion for books worth reading.

Send a photo of a page and I'll analyse it: summary, vocabulary, notable quotes, what you might have missed, and connections to what came before.

/book Title — Author   set the book you're reading
/books                 browse your past analyses
/budget                check today's remaining analyses
/status                see the current book
/quotes                all quotes collected for this book`

const noBookForPhotoReply = "Set a book first with /book Title — Author"

const imageRetryReply = "That image is unreadable — too blurry, too dark, or cut off. Try again with better lighting and the full page in frame."

const imageDownloadFailedReply = "That image didn't come through. Try resending it."

const claudeErrorReply = "The analysis engine is being difficult. Try again in a moment."

const claudeJSONErrorReply = "Got a response but it came back garbled. Twice. Send the page again — if this continues, something deeper is broken."

const callbackExpiredReply = "That analysis couldn't be found. Send the page again."

func bookParseErrorReply() string {
	return "I need both a title and an author, separated by a dash. Like this:\n/book The Brothers Karamazov - Dostoevsky"
}

func formatVocabSection(entries []analysis.VocabEntry) string {
	if len(entries) == 0 {
		return "No tricky vocabulary on this page."
	}
	var b strings.Builder
	b.WriteString("<b>Vocabulary</b>\n\n")
	for _, v := range entries {
		b.WriteString(fmt.Sprintf("<b>%s</b> — %s\n", escapeHTML(v.Word), escapeHTML(v.Definition)))
		b.WriteString(fmt.Sprintf("<i>%s</i>\n\n", escapeHTML(v.Sentence)))
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatQuotesSection(entries []analysis.Quote) string {
	if len(entries) == 0 {
		return "Nothing on this page that demanded to be underlined."
	}
	var b strings.Builder
	b.WriteString("<b>Quotes</b>\n\n")
	for _, q := range entries {
		b.WriteString(fmt.Sprintf("\u201c%s\u201d\n", escapeHTML(q.Text)))
		b.WriteString(fmt.Sprintf("&#x2014; %s\n\n", escapeHTML(q.Note)))
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatMissedSection(missed string) string {
	if missed == "" {
		return "Nothing flagged for this page."
	}
	var b strings.Builder
	b.WriteString("<b>What You Might Have Missed</b>\n\n")
	b.WriteString(escapeHTML(missed))
	return b.String()
}

func formatConnectionsSection(connections string) string {
	if connections == "" {
		return "No connections to earlier pages this time."
	}
	var b strings.Builder
	b.WriteString("<b>Connections to Earlier Pages</b>\n\n")
	b.WriteString(escapeHTML(connections))
	return b.String()
}

const maxTelegramMessage = 4000

func formatAllQuotes(title string, quotes []analysis.Quote) []string {
	if len(quotes) == 0 {
		return []string{fmt.Sprintf("No quotes collected from \"%s\" yet.", escapeHTML(title))}
	}

	header := fmt.Sprintf("<b>Quotes from \"%s\"</b> (%d total)\n\n", escapeHTML(title), len(quotes))

	var messages []string
	var current strings.Builder
	current.WriteString(header)

	for _, q := range quotes {
		entry := fmt.Sprintf("\u201c%s\u201d\n&#x2014; %s\n\n", escapeHTML(q.Text), escapeHTML(q.Note))

		if current.Len()+len(entry) > maxTelegramMessage {
			messages = append(messages, current.String())
			current.Reset()
			current.WriteString("<b>Quotes (continued)</b>\n\n")
		}
		current.WriteString(entry)
	}

	if current.Len() > 0 {
		messages = append(messages, current.String())
	}

	return messages
}

// progressBar returns a 15-block ASCII bar showing filled/total proportion.
// e.g. progressBar(7, 15) → "███████░░░░░░░░"
func progressBar(used, total int) string {
	const barLen = 15
	if total <= 0 {
		return strings.Repeat("░", barLen)
	}
	filled := used * barLen / total
	if filled > barLen {
		filled = barLen
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", barLen-filled)
}

func formatBudget(used, limit int) string {
	remaining := limit - used
	if remaining < 0 {
		remaining = 0
	}
	pct := used * 100 / limit
	bar := progressBar(used, limit)
	return fmt.Sprintf(
		"Today's reading\n\n%s %d%%\n%d of %d analyses used · %d remaining\n\nUp to 4 pages per analysis · resets at midnight UTC",
		bar, pct, used, limit, remaining,
	)
}

// formatAllQuotesGrouped renders quotes from all books, one section per book.
func formatAllQuotesGrouped(groups []storage.BookQuotes) []string {
	if len(groups) == 0 {
		return []string{"No quotes collected yet. They accumulate as you analyse pages."}
	}

	total := 0
	for _, g := range groups {
		total += len(g.Quotes)
	}

	var messages []string
	var current strings.Builder
	current.WriteString(fmt.Sprintf("<b>Your Quotes</b>  ·  %d total\n", total))

	for _, g := range groups {
		bookHeader := fmt.Sprintf("\n\n<b>%s</b>", escapeHTML(g.BookTitle))
		if g.BookAuthor != "" {
			bookHeader += fmt.Sprintf(" — <i>%s</i>", escapeHTML(g.BookAuthor))
		}
		bookHeader += "\n"

		if current.Len()+len(bookHeader) > maxTelegramMessage {
			messages = append(messages, strings.TrimRight(current.String(), "\n"))
			current.Reset()
		}
		current.WriteString(bookHeader)

		for _, q := range g.Quotes {
			entry := fmt.Sprintf("\n\u201c%s\u201d\n<i>%s</i>\n", escapeHTML(q.Text), escapeHTML(q.Note))
			if current.Len()+len(entry) > maxTelegramMessage {
				messages = append(messages, strings.TrimRight(current.String(), "\n"))
				current.Reset()
				current.WriteString("<b>Quotes (continued)</b>\n")
			}
			current.WriteString(entry)
		}
	}

	if current.Len() > 0 {
		messages = append(messages, strings.TrimRight(current.String(), "\n"))
	}
	return messages
}

func escapeHTML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return r.Replace(s)
}
