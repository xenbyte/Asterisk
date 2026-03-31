package telegram

import (
	"fmt"
	"strings"

	"github.com/xenbyte/asterisk/internal/analysis"
)

func bookSetReply(title, author string) string {
	replies := []string{
		fmt.Sprintf("Got it. %s by %s. Let's see what we're getting into.", title, author),
		fmt.Sprintf("%s by %s. Good choice — or at least an interesting one. Let's go.", title, author),
		fmt.Sprintf("Noted. %s, %s. I have thoughts about this one, but I'll save them.", title, author),
	}
	return replies[len(title)%len(replies)]
}

func statusReply(title, author string) string {
	return fmt.Sprintf("Currently reading: %s by %s.", title, author)
}

const statusNoBookReply = "No book set. Use /book <title> - <author> to tell me what we're reading. I can't analyze pages if I don't know the book."

const helpReply = `Here's how this works.

You tell me what you're reading with /book — for example:
/book The Brothers Karamazov - Dostoevsky

Then you send me photos of pages. I'll read them and give you:
• A summary of what actually happens on the page
• Vocabulary that might trip you up
• Quotes worth keeping
• Things you probably missed — allusions, subtext, the stuff authors assume you already know
• Connections to pages you've already sent — threads developing across the book

Use /status to check what book is active.
Use /quotes to see every quote collected from the current book so far.

That's it. Send pages, I'll read with you.`

const noBookForPhotoReply = "I need to know what book this is from before I can do anything useful with it. Use /book <title> - <author> first."

const imageRetryReply = "I can barely read that. The image is too blurry, too dark, or cut off. Try again — better lighting, steady hands, the whole page in frame."

const imageDownloadFailedReply = "That image didn't come through cleanly. Could be too large or Telegram had a moment. Try resending it, maybe at a lower resolution."

const claudeErrorReply = "Something went wrong on my end — the analysis engine is being difficult. Give it a minute and try again. If it keeps happening, the API might be down."

const claudeJSONErrorReply = "I got a response but it came back garbled. Twice. That's unusual. Try sending the page again — if this keeps up, something deeper is broken."

const callbackExpiredReply = "That analysis couldn't be found. Send the page again and I'll re-read it."

func bookParseErrorReply() string {
	return "I need both a title and an author, separated by a dash. Like this:\n/book The Brothers Karamazov - Dostoevsky"
}

func formatVocabSection(entries []analysis.VocabEntry) string {
	if len(entries) == 0 {
		return "No tricky vocabulary on this page."
	}
	var b strings.Builder
	b.WriteString("📖 <b>Vocabulary</b>\n\n")
	for _, v := range entries {
		b.WriteString(fmt.Sprintf("<b>%s</b> — %s\n", escapeHTML(v.Word), escapeHTML(v.Definition)))
		b.WriteString(fmt.Sprintf("<i>%s</i>\n\n", escapeHTML(v.SentenceFromText)))
	}
	return b.String()
}

func formatQuotesSection(entries []analysis.QuoteEntry) string {
	if len(entries) == 0 {
		return "Nothing on this page that demanded to be underlined."
	}
	var b strings.Builder
	b.WriteString("💬 <b>Quotes</b>\n\n")
	for _, q := range entries {
		b.WriteString(fmt.Sprintf("\u201c%s\u201d\n", escapeHTML(q.Quote)))
		b.WriteString(fmt.Sprintf("↳ %s\n\n", escapeHTML(q.WhyItMatters)))
	}
	return b.String()
}

func formatMissedSection(entries []string) string {
	if len(entries) == 0 {
		return "This page is fairly straightforward — nothing hidden that I'd flag."
	}
	var b strings.Builder
	b.WriteString("🔍 <b>What You Might Have Missed</b>\n\n")
	for i, m := range entries {
		b.WriteString(fmt.Sprintf("%d. %s\n\n", i+1, escapeHTML(m)))
	}
	return b.String()
}

func formatConnectionsSection(entries []analysis.Connection) string {
	if len(entries) == 0 {
		return "No connections to earlier pages this time — either this is fresh ground or I need more context to work with."
	}
	var b strings.Builder
	b.WriteString("🔗 <b>Connections to Earlier Pages</b>\n\n")
	for i, c := range entries {
		b.WriteString(fmt.Sprintf("<b>%d.</b> %s\n", i+1, escapeHTML(c.Insight)))
		b.WriteString(fmt.Sprintf("   <i>Now:</i> %s\n", escapeHTML(c.CurrentText)))
		b.WriteString(fmt.Sprintf("   <i>Earlier:</i> %s\n\n", escapeHTML(c.ConnectsTo)))
	}
	return b.String()
}

const maxTelegramMessage = 4000

func formatAllQuotes(title string, quotes []analysis.QuoteEntry) []string {
	if len(quotes) == 0 {
		return []string{fmt.Sprintf("No quotes collected from \"%s\" yet. Send some pages and I'll start keeping track.", title)}
	}

	header := fmt.Sprintf("💬 <b>All Quotes from \"%s\"</b> (%d total)\n\n", escapeHTML(title), len(quotes))

	var messages []string
	var current strings.Builder
	current.WriteString(header)

	for _, q := range quotes {
		entry := fmt.Sprintf("\u201c%s\u201d\n↳ %s\n\n", escapeHTML(q.Quote), escapeHTML(q.WhyItMatters))

		if current.Len()+len(entry) > maxTelegramMessage {
			messages = append(messages, current.String())
			current.Reset()
			current.WriteString("💬 <b>Quotes (continued)</b>\n\n")
		}
		current.WriteString(entry)
	}

	if current.Len() > 0 {
		messages = append(messages, current.String())
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
