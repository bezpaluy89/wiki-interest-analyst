// Package wiki is a thin, cached client for the public Wikimedia APIs used by
// the skill: the Pageviews REST API (article view counts over time) and the
// MediaWiki Action API (resolving a topic to per-language article titles).
//
// All requests send a descriptive User-Agent, which Wikimedia requires — calls
// without one are rejected with HTTP 403/429. Responses are cached on disk so
// repeated and related queries are fast, cheap and reproducible.
package wiki

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// DefaultUserAgent identifies the tool to Wikimedia. It intentionally carries
// no personal data (the case task requires anonymity). Override via NewClient.
const DefaultUserAgent = "wiki-interest-analyst/1.0 (Agent Skill; +https://github.com/wiki-interest-analyst)"

// Default API base URLs. Overridable on Client for testing.
const (
	defaultPageviewBase = "https://wikimedia.org/api/rest_v1/metrics/pageviews/per-article"
	defaultActionBase   = "https://%s.wikipedia.org/w/api.php" // %s = language code
)

// Client talks to Wikimedia with an on-disk response cache.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	CacheDir  string
	TTL       time.Duration

	PageviewBase string
	ActionBase   string
}

func NewClient(cacheDir string) *Client {
	return &Client{
		HTTP:         &http.Client{Timeout: 30 * time.Second},
		UserAgent:    DefaultUserAgent,
		CacheDir:     cacheDir,
		TTL:          7 * 24 * time.Hour,
		PageviewBase: defaultPageviewBase,
		ActionBase:   defaultActionBase,
	}
}

// getJSON fetches url and returns the raw body, using the cache when possible.
// It retries transient failures with backoff and surfaces Wikimedia's own error
// bodies so callers (and the agent) get an actionable message.
func (c *Client) getJSON(url string) ([]byte, error) {
	if body, ok := c.readCache(url); ok {
		return body, nil
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 400 * time.Millisecond)
		}

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("User-Agent", c.UserAgent)
		req.Header.Set("Accept", "application/json")

		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err

			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr

			continue
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			c.writeCache(url, body)

			return body, nil
		case resp.StatusCode == http.StatusNotFound:
			return body, nil
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("wikimedia %d: %s", resp.StatusCode, snippet(body))

			continue
		default:
			return nil, fmt.Errorf("wikimedia %d: %s", resp.StatusCode, snippet(body))
		}
	}

	return nil, fmt.Errorf("request failed after retries: %w", lastErr)
}

func snippet(b []byte) string {
	const max = 200
	if len(b) > max {
		return string(b[:max]) + "…"
	}

	return string(b)
}

func (c *Client) cachePath(url string) string {
	sum := sha1.Sum([]byte(url))

	return filepath.Join(c.CacheDir, hex.EncodeToString(sum[:])+".json")
}

func (c *Client) readCache(url string) ([]byte, bool) {
	if c.CacheDir == "" {
		return nil, false
	}

	p := c.cachePath(url)
	info, err := os.Stat(p)
	if err != nil {
		return nil, false
	}

	if c.TTL > 0 && time.Since(info.ModTime()) > c.TTL {
		return nil, false
	}

	body, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}

	return body, true
}

func (c *Client) writeCache(url string, body []byte) {
	if c.CacheDir == "" {
		return
	}

	if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
		return
	}

	// Best-effort: a cache write failure must never break a query.
	_ = os.WriteFile(c.cachePath(url), body, 0o644)
}
