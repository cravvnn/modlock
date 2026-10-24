package session

import (
	"sync"

	"github.com/paralin/modlock/src/host_app/control"
)

// chatKept is how many of the newest lines Chat keeps for a slow stream.
const chatKept = 64

// Chat keeps the newest lines said on a server, numbered in order, for
// streams that send each line once. A stream that falls more than chatKept
// lines behind skips the oldest.
type Chat struct {
	// mu guards the fields below.
	mu sync.Mutex
	// lines holds the newest lines, oldest first.
	lines []*control.PlayerChat
	// next is the number the next line said gets.
	next uint64
	// changed is closed and replaced whenever a line is said.
	changed chan struct{}
}

// NewChat returns a chat with no lines.
func NewChat() *Chat {
	return &Chat{changed: make(chan struct{})}
}

// Add records a line and wakes every stream.
func (c *Chat) Add(line *control.PlayerChat) {
	// Keep the newest lines, then wake every stream.
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, line)
	if len(c.lines) > chatKept {
		c.lines = c.lines[len(c.lines)-chatKept:]
	}
	c.next++
	close(c.changed)
	c.changed = make(chan struct{})
}

// Since returns the lines kept from number from on, the number of the next
// line, and a channel that closes when another line is said. A stream that
// starts at the next line passes the largest number.
func (c *Chat) Since(from uint64) ([]*control.PlayerChat, uint64, <-chan struct{}) {
	// Clamp from to the lines kept.
	c.mu.Lock()
	defer c.mu.Unlock()
	first := c.next - uint64(len(c.lines))
	from = min(max(from, first), c.next)
	return c.lines[from-first:], c.next, c.changed
}
