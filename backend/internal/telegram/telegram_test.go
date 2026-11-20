package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendOmitsThreadWhenNil(t *testing.T) {
	t.Parallel()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	t.Cleanup(srv.Close)

	c := New("test-token", srv.URL, nil)
	if err := c.Send(context.Background(), "-1003787521365", "hello", nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["message_thread_id"]; ok {
		t.Fatalf("expected no message_thread_id, got %#v", got)
	}
	if got["chat_id"] != "-1003787521365" || got["text"] != "hello" {
		t.Fatalf("unexpected payload: %#v", got)
	}
}

func TestSendIncludesPositiveThread(t *testing.T) {
	t.Parallel()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	t.Cleanup(srv.Close)

	thread := int64(599)
	c := New("test-token", srv.URL, nil)
	if err := c.Send(context.Background(), "-1003787521365", "topic hello", &thread); err != nil {
		t.Fatal(err)
	}
	// JSON numbers decode as float64 in map[string]any.
	if got["message_thread_id"] != float64(599) {
		t.Fatalf("message_thread_id = %#v, want 599", got["message_thread_id"])
	}
}

func TestSendIgnoresNonPositiveThread(t *testing.T) {
	t.Parallel()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	t.Cleanup(srv.Close)

	zero := int64(0)
	c := New("test-token", srv.URL, nil)
	if err := c.Send(context.Background(), "1", "x", &zero); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["message_thread_id"]; ok {
		t.Fatalf("expected no message_thread_id for 0, got %#v", got)
	}
}
