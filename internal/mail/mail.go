package mail

import (
	"fmt"
	"strings"

	imap "github.com/BrianLeishman/go-imap"
)

// Message is a slim representation of a fetched mail with its attachments.
type Message struct {
	UID         int
	Attachments []Attachment
}

// Attachment holds a single mail attachment's name and raw content.
type Attachment struct {
	Name    string
	Content []byte
}

// Client wraps the go-imap Dialer with the operations the processor needs.
type Client struct {
	d      *imap.Dialer
	idleCh chan struct{}
}

// Connect establishes a TLS IMAP connection and authenticates via LOGIN.
func Connect(server string, port int, username, password string) (*Client, error) {
	d, err := imap.New(username, password, server, port)
	if err != nil {
		return nil, fmt.Errorf("imap connect to %s:%d: %w", server, port, err)
	}
	return &Client{d: d, idleCh: make(chan struct{}, 1)}, nil
}

// SelectFolder opens the named IMAP folder for reading.
func (c *Client) SelectFolder(name string) error {
	if err := c.d.SelectFolder(name); err != nil {
		return fmt.Errorf("select folder %q: %w", name, err)
	}
	return nil
}

// SearchUIDs returns UIDs matching the given constraints.
// minUID=0 returns all messages; minUID>0 restricts to UIDs >= minUID.
// sender may be empty to skip the FROM filter.
func (c *Client) SearchUIDs(minUID int, sender string) ([]int, error) {
	criteria := buildCriteria(minUID, sender)
	uids, err := c.d.GetUIDs(criteria)
	if err != nil {
		// Some servers return NO instead of an empty set when the UID range
		// has no matches; treat that as an empty result.
		if strings.Contains(err.Error(), "BAD") || strings.Contains(err.Error(), "NO") {
			return nil, nil
		}
		return nil, fmt.Errorf("search UIDs (criteria=%q): %w", criteria, err)
	}
	// Guard against a server that ignores the UID lower bound.
	if minUID > 0 {
		filtered := uids[:0]
		for _, uid := range uids {
			if uid >= minUID {
				filtered = append(filtered, uid)
			}
		}
		return filtered, nil
	}
	return uids, nil
}

func buildCriteria(minUID int, sender string) string {
	var parts []string
	if minUID > 0 {
		parts = append(parts, fmt.Sprintf("UID %d:*", minUID))
	}
	if sender != "" {
		parts = append(parts, fmt.Sprintf("FROM %q", sender))
	}
	if len(parts) == 0 {
		return "ALL"
	}
	return strings.Join(parts, " ")
}

// FetchMessages retrieves the full content (including attachments) for the
// given UIDs. Returns nil, nil when uids is empty.
func (c *Client) FetchMessages(uids []int) ([]*Message, error) {
	if len(uids) == 0 {
		return nil, nil
	}
	emails, err := c.d.GetEmails(uids...)
	if err != nil {
		return nil, fmt.Errorf("fetch %d messages: %w", len(uids), err)
	}

	msgs := make([]*Message, 0, len(emails))
	for uid, email := range emails {
		msg := &Message{UID: uid}
		for _, a := range email.Attachments {
			msg.Attachments = append(msg.Attachments, Attachment{
				Name:    a.Name,
				Content: a.Content,
			})
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// StartIdle begins IMAP IDLE. New-message events are signalled on IdleEvents().
// The same channel is reused across Stop/Start cycles.
func (c *Client) StartIdle() error {
	ch := c.idleCh
	handler := &imap.IdleHandler{
		OnExists: func(_ imap.ExistsEvent) {
			select {
			case ch <- struct{}{}:
			default: // already pending, do not block
			}
		},
	}
	if err := c.d.StartIdle(handler); err != nil {
		return fmt.Errorf("start idle: %w", err)
	}
	return nil
}

// StopIdle terminates the active IDLE session.
func (c *Client) StopIdle() error {
	if err := c.d.StopIdle(); err != nil {
		return fmt.Errorf("stop idle: %w", err)
	}
	return nil
}

// IdleEvents returns the channel that receives a signal whenever EXISTS fires.
func (c *Client) IdleEvents() <-chan struct{} {
	return c.idleCh
}

// Close tears down the IMAP connection.
func (c *Client) Close() {
	c.d.Close()
}
