package telegram

import (
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// mediaGroupBuffer collects photos that arrive as part of the same Telegram
// media group (user sent multiple photos at once). After a short quiet period
// with no new photos for the group, it fires the callback with all collected
// messages.
type mediaGroupBuffer struct {
	mu       sync.Mutex
	groups   map[string]*pendingGroup
	debounce time.Duration
}

type pendingGroup struct {
	chatID   int64
	messages []*tgbotapi.Message
	timer    *time.Timer
}

func newMediaGroupBuffer(debounce time.Duration) *mediaGroupBuffer {
	return &mediaGroupBuffer{
		groups:   make(map[string]*pendingGroup),
		debounce: debounce,
	}
}

// add buffers a photo message. If this is the last photo in the group (no more
// arrive within the debounce window), onReady is called with all collected
// messages. onReady is called in a new goroutine.
func (b *mediaGroupBuffer) add(msg *tgbotapi.Message, onReady func(chatID int64, msgs []*tgbotapi.Message)) {
	b.mu.Lock()
	defer b.mu.Unlock()

	groupID := msg.MediaGroupID
	pg, exists := b.groups[groupID]

	if exists {
		pg.messages = append(pg.messages, msg)
		pg.timer.Reset(b.debounce)
		return
	}

	pg = &pendingGroup{
		chatID:   msg.Chat.ID,
		messages: []*tgbotapi.Message{msg},
	}

	pg.timer = time.AfterFunc(b.debounce, func() {
		b.mu.Lock()
		group := b.groups[groupID]
		delete(b.groups, groupID)
		b.mu.Unlock()

		if group != nil {
			onReady(group.chatID, group.messages)
		}
	})

	b.groups[groupID] = pg
}
