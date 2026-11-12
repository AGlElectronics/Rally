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
)

// Client sends messages with one bot token. A Client with an empty token is
// a dry run: Send succeeds without contacting Telegram.
type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// New returns a client; baseURL defaults to the public Bot API.
func New(token, baseURL string) *Client {
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	return &Client{
		token:   strings.TrimSpace(token),
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Configured reports whether a bot token is set.
func (c *Client) Configured() bool { return c != nil && c.token != "" }

// Send posts text to chatID (a numeric id or @channelname).
func (c *Client) Send(ctx context.Context, chatID, text string) error {
	if !c.Configured() {
		return nil
	}
	body, err := json.Marshal(map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/bot"+c.token+"/sendMessage", bytes.NewReader(body))
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
