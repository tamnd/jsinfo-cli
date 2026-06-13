// Package jsinfo is the library behind the jsinfo command: the HTTP client,
// request shaping, and the typed data models for javascript.info.
//
// The Client fetches the tutorial's table of contents from the main page,
// parsing part names and article links without any third-party HTML library.
// It uses the standard library only (strings, net/http, context).
package jsinfo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultUserAgent identifies the client to javascript.info.
const DefaultUserAgent = "jsinfo-cli/dev (+https://github.com/tamnd/jsinfo-cli)"

// Config holds constructor parameters for the client.
type Config struct {
	BaseURL   string
	Rate      time.Duration
	Retries   int
	UserAgent string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://javascript.info",
		Rate:      300 * time.Millisecond,
		Retries:   5,
		UserAgent: DefaultUserAgent,
	}
}

// Article is one tutorial article or chapter in the javascript.info TOC.
type Article struct {
	Title string `json:"title"`
	Part  string `json:"part"`
	URL   string `json:"url"`
}

// Client talks to javascript.info over HTTP.
type Client struct {
	http      *http.Client
	cfg       Config
	last      time.Time
}

// NewClient returns a Client configured by cfg.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultConfig().BaseURL
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultConfig().UserAgent
	}
	if cfg.Retries == 0 {
		cfg.Retries = DefaultConfig().Retries
	}
	return &Client{
		http: &http.Client{Timeout: 30 * time.Second},
		cfg:  cfg,
	}
}

// Contents fetches the javascript.info homepage and returns the list of
// tutorial articles in TOC order. limit <= 0 means no limit.
func (c *Client) Contents(ctx context.Context, limit int) ([]Article, error) {
	body, err := c.get(ctx, c.cfg.BaseURL+"/")
	if err != nil {
		return nil, fmt.Errorf("fetch homepage: %w", err)
	}
	articles := parseTOC(string(body), c.cfg.BaseURL)
	if limit > 0 && len(articles) > limit {
		articles = articles[:limit]
	}
	return articles, nil
}

// Search returns articles whose Title or Part contains query (case-insensitive).
// It fetches the full TOC then filters in memory. limit <= 0 means no limit.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Article, error) {
	all, err := c.Contents(ctx, 0)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var out []Article
	for _, a := range all {
		if strings.Contains(strings.ToLower(a.Title), q) ||
			strings.Contains(strings.ToLower(a.Part), q) {
			out = append(out, a)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

// get fetches url and returns the body, pacing and retrying on transient errors.
func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", url, lastErr)
}

func (c *Client) do(ctx context.Context, url string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace enforces the minimum gap between requests.
func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// parseTOC parses the javascript.info homepage HTML and extracts articles
// grouped by part. It uses only stdlib strings — no x/net/html.
//
// The page structure is:
//
//	<h2>The JavaScript language</h2>
//	  ... links to chapters and sections ...
//	<h2>Browser: Document, Events, Interfaces</h2>
//	  ...
//
// Each chapter link looks like: href="/slug">Title</a>
// We skip /task/ paths (exercise links) and other non-chapter paths.
func parseTOC(html, baseURL string) []Article {
	// Locate the "Table of contents" anchor and work from there.
	start := strings.Index(html, "Table of contents")
	if start < 0 {
		start = 0
	}
	body := html[start:]

	// Split on <h2 to get one chunk per part.
	parts := strings.Split(body, "<h2")
	// parts[0] is before the first <h2 (the "Table of contents" preamble)
	// parts[1..] each start with the rest of the h2 tag.

	// Known non-tutorial parts we can skip.
	skipParts := map[string]bool{
		"Watch for javascript.info updates": true,
		"": true,
	}

	var articles []Article

	for _, part := range parts[1:] {
		// Extract h2 text: everything before </h2>.
		// After splitting on "<h2", 'part' starts with " class="...">text</h2> ..."
		// We skip past the first ">" (end of the opening tag) to get the h2 inner text.
		h2End := strings.Index(part, "</h2>")
		if h2End < 0 {
			continue
		}
		h2Raw := part[:h2End]
		// Skip past the end of the opening tag's attribute list.
		if gt := strings.IndexByte(h2Raw, '>'); gt >= 0 {
			h2Raw = h2Raw[gt+1:]
		}
		partName := innerText(h2Raw)
		if skipParts[partName] {
			continue
		}
		// Only process the three known tutorial parts (stop before footer).
		if !isTutorialPart(partName) {
			continue
		}

		// Find the next <h2 boundary (or end of string) to scope our search.
		// We already split on <h2, so the content for this part is all of 'part'
		// up to the next occurrence that we'd process separately.
		// Actually since we split on <h2, 'part' already ends before the next h2.
		// Just scan 'part' for article links.
		chunkEnd := len(part)
		_ = chunkEnd

		// Walk through all href="..." in this chunk.
		chunk := part[h2End:]
		for {
			hrefIdx := strings.Index(chunk, `href="`)
			if hrefIdx < 0 {
				break
			}
			chunk = chunk[hrefIdx+6:] // skip past href="
			end := strings.IndexByte(chunk, '"')
			if end < 0 {
				break
			}
			href := chunk[:end]
			chunk = chunk[end+1:]

			// Only relative tutorial paths: /slug (no sub-paths, no tasks).
			if !isChapterHref(href) {
				continue
			}

			// Grab the link text: find the > after the href attr, then </a>.
			gtIdx := strings.IndexByte(chunk, '>')
			if gtIdx < 0 {
				continue
			}
			afterGt := chunk[gtIdx+1:]
			closeIdx := strings.Index(afterGt, "</a>")
			if closeIdx < 0 {
				continue
			}
			titleRaw := afterGt[:closeIdx]
			title := innerText(titleRaw)
			title = unescapeHTML(title)
			if title == "" {
				continue
			}

			articles = append(articles, Article{
				Title: title,
				Part:  partName,
				URL:   baseURL + href,
			})
		}
	}
	return articles
}

// isTutorialPart returns true for the three main tutorial parts.
func isTutorialPart(name string) bool {
	switch name {
	case "The JavaScript language",
		"Browser: Document, Events, Interfaces",
		"Additional articles":
		return true
	}
	return false
}

// isChapterHref returns true for /slug paths that are tutorial chapters.
// Rejects: external URLs, /task/ paths, single-char paths, anchors.
func isChapterHref(href string) bool {
	if href == "" || href[0] != '/' {
		return false
	}
	if strings.HasPrefix(href, "//") {
		return false
	}
	if strings.Contains(href, "#") {
		return false
	}
	if strings.HasPrefix(href, "/task/") {
		return false
	}
	// Must be a single-level path like /hello-world (no trailing slash, no sub-paths)
	// Allow /slug only.
	rest := href[1:]
	if rest == "" {
		return false
	}
	if strings.Contains(rest, "/") {
		return false
	}
	return true
}

// innerText strips all HTML tags from s and returns the plain text.
func innerText(s string) string {
	var b strings.Builder
	inTag := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteByte(s[i])
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// unescapeHTML replaces common HTML entities in s.
func unescapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&apos;", "'")
	s = strings.ReplaceAll(s, "&#34;", `"`)
	return s
}
