// Package embedding is a thin HTTP client for the local embedding service
// (embed_server.py / bge-base-zh). Optional: a nil Client makes Embed return
// (nil, nil) so RAG falls back to keyword (ILIKE) search when the service is
// unavailable or EMBED_URL is unset.
package embedding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type Client struct {
	url  string
	http *http.Client
}

// NewFromEnv builds a client from EMBED_URL (default http://127.0.0.1:8090).
func NewFromEnv() *Client {
	u := os.Getenv("EMBED_URL")
	if u == "" {
		u = "http://127.0.0.1:8090"
	}
	return &Client{url: u, http: &http.Client{Timeout: 60 * time.Second}}
}

// Embed returns one vector per input text. nil client or empty input → (nil, nil).
func (c *Client) Embed(texts []string) ([][]float32, error) {
	if c == nil || len(texts) == 0 {
		return nil, nil
	}
	body, _ := json.Marshal(map[string]any{"inputs": texts})
	resp, err := c.http.Post(c.url+"/embed", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embed service %d: %s", resp.StatusCode, string(raw))
	}
	var out [][]float32
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
