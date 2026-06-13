package jsinfo_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/jsinfo-cli/jsinfo"
)

// minimalHomepage is a trimmed replica of the javascript.info homepage
// structure used to test TOC parsing without real network calls.
const minimalHomepage = `<!DOCTYPE html>
<html>
<head><title>Javascript.info</title></head>
<body>
<h2>Table of contents</h2>
<h2>The JavaScript language</h2>
<ul>
  <li><a href="/getting-started">An introduction</a></li>
  <li><a href="/intro">An Introduction to JavaScript</a></li>
  <li><a href="/first-steps">JavaScript Fundamentals</a></li>
  <li><a href="/hello-world">Hello, world!</a></li>
  <li><a href="/task/some-task">task (should be skipped)</a></li>
  <li><a href="/object-basics">Objects: the basics</a></li>
</ul>
<h2>Browser: Document, Events, Interfaces</h2>
<ul>
  <li><a href="/document">Document</a></li>
  <li><a href="/browser-environment">Browser environment, specs</a></li>
</ul>
<h2>Additional articles</h2>
<ul>
  <li><a href="/frames-and-windows">Frames and windows</a></li>
</ul>
<h2>Watch for javascript.info updates</h2>
<p>footer stuff</p>
</body>
</html>`

func TestContents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("no User-Agent header sent")
		}
		fmt.Fprint(w, minimalHomepage)
	}))
	defer srv.Close()

	cfg := jsinfo.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 0
	c := jsinfo.NewClient(cfg)

	articles, err := c.Contents(context.Background(), 0)
	if err != nil {
		t.Fatalf("Contents: %v", err)
	}
	if len(articles) == 0 {
		t.Fatal("Contents returned no articles")
	}

	// Verify task links are excluded.
	for _, a := range articles {
		if strings.Contains(a.URL, "/task/") {
			t.Errorf("task link leaked into results: %v", a.URL)
		}
	}

	// Check a known article is present.
	found := false
	for _, a := range articles {
		if a.Title == "Hello, world!" {
			found = true
			if a.Part != "The JavaScript language" {
				t.Errorf("wrong part %q for Hello world", a.Part)
			}
			wantURL := srv.URL + "/hello-world"
			if a.URL != wantURL {
				t.Errorf("URL = %q, want %q", a.URL, wantURL)
			}
		}
	}
	if !found {
		t.Error("expected to find 'Hello, world!' article")
	}

	// Check cross-part: browser article.
	foundBrowser := false
	for _, a := range articles {
		if a.Title == "Document" && a.Part == "Browser: Document, Events, Interfaces" {
			foundBrowser = true
		}
	}
	if !foundBrowser {
		t.Error("expected to find 'Document' in browser part")
	}
}

func TestContentsLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, minimalHomepage)
	}))
	defer srv.Close()

	cfg := jsinfo.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 0
	c := jsinfo.NewClient(cfg)

	articles, err := c.Contents(context.Background(), 3)
	if err != nil {
		t.Fatalf("Contents: %v", err)
	}
	if len(articles) != 3 {
		t.Errorf("want 3 articles, got %d", len(articles))
	}
}

func TestSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, minimalHomepage)
	}))
	defer srv.Close()

	cfg := jsinfo.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 0
	c := jsinfo.NewClient(cfg)

	results, err := c.Search(context.Background(), "object", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search returned no results for 'object'")
	}
	for _, a := range results {
		if !strings.Contains(strings.ToLower(a.Title), "object") &&
			!strings.Contains(strings.ToLower(a.Part), "object") {
			t.Errorf("result %q does not match query 'object'", a.Title)
		}
	}
}

func TestSearchLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, minimalHomepage)
	}))
	defer srv.Close()

	cfg := jsinfo.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 0
	c := jsinfo.NewClient(cfg)

	// Search for something that matches multiple articles (part name match).
	results, err := c.Search(context.Background(), "javascript", 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) > 2 {
		t.Errorf("want at most 2 results, got %d", len(results))
	}
}

func TestRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, minimalHomepage)
	}))
	defer srv.Close()

	cfg := jsinfo.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := jsinfo.NewClient(cfg)

	_, err := c.Contents(context.Background(), 0)
	if err != nil {
		t.Fatalf("expected retry to succeed: %v", err)
	}
	if hits != 3 {
		t.Errorf("expected 3 server hits, got %d", hits)
	}
}
