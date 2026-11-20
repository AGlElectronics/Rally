// Package telegram posts plain-text messages through the Telegram Bot API.
// Only sendMessage is used; the bot is a one-way announcer.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/AGlElectronics/rally/backend/internal/tgcatchall"
)

// Client sends messages with one bot token. A Client with an empty token is
// a dry run: Send succeeds without contacting Telegram.
type Client struct {
	token    string
	baseURL  string
	http     *http.Client
	catchall *tgcatchall.Client
}

// New returns a client; baseURL defaults to the public Bot API.
// catchall, when enabled (Compose / APP_ENV=development), redirects every
// send to one test chat with a prefix naming the real target.
func New(token, baseURL string, catchall *tgcatchall.Client) *Client {
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	return &Client{
		token:    strings.TrimSpace(token),
		baseURL:  strings.TrimRight(baseURL, "/"),
		http:     &http.Client{Timeout: 15 * time.Second},
		catchall: catchall,
	}
}

// Configured reports whether a bot token is set, or a dev catch-all is on.
func (c *Client) Configured() bool {
	return c != nil && (c.token != "" || c.catchall.Enabled())
}

// Send posts text to chatID (a numeric id or @channelname). When threadID
// is set and positive, the message goes to that forum topic
// (message_thread_id). Nil or non-positive omits the field (whole chat /
// General). With a dev catch-all, the message goes there instead, prefixed
// with the intended chat/topic.
func (c *Client) Send(ctx context.Context, chatID, text string, threadID *int64) error {
	if c.catchall.Enabled() {
		intended := "Rally chat " + chatID
		if threadID != nil && *threadID > 0 {
			intended = fmt.Sprintf("Rally chat %s topic %d", chatID, *threadID)
		}
		return c.catchall.Send(ctx, intended, text)
	}
	if c == nil || c.token == "" {
		return nil
	}
	body := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	}
	if threadID != nil && *threadID > 0 {
		body["message_thread_id"] = *threadID
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/bot"+c.token+"/sendMessage", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		// The URL carries the token; never surface it.
		return fmt.Errorf("telegram unreachable")
	}
	defer func() { _ = res.Body.Close() }()
	var out struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	if !out.OK {
		if out.Description == "" {
			out.Description = res.Status
		}
		return fmt.Errorf("telegram: %s", out.Description)
	}
	return nil
}
