// Package mavencentral is the library behind the mavencentral command line:
// the HTTP client, request shaping, and the typed data models for Maven Central.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public site throws under load.
package mavencentral

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// DefaultUserAgent identifies the client to Maven Central. A real, honest
// User-Agent is both polite and the thing most likely to keep you unblocked.
const DefaultUserAgent = "mavencentral-cli/dev (+https://github.com/tamnd/mavencentral-cli)"

// Host is the site this client talks to, and the host the URI driver in
// domain.go claims.
const Host = "search.maven.org"

// BaseURL is the root every request is built from.
const BaseURL = "https://search.maven.org/solrsearch"

// Config holds the tunable settings for the client.
type Config struct {
	BaseURL string
	Rate    time.Duration
	Retries int
	Timeout time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL: "https://search.maven.org/solrsearch",
		Rate:    300 * time.Millisecond,
		Retries: 3,
		Timeout: 15 * time.Second,
	}
}

// Client talks to Maven Central over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	BaseURL   string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults.
func NewClient() *Client {
	cfg := DefaultConfig()
	return &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: DefaultUserAgent,
		BaseURL:   cfg.BaseURL,
		Rate:      cfg.Rate,
		Retries:   cfg.Retries,
	}
}

// NewClientFromConfig returns a Client configured from cfg.
func NewClientFromConfig(cfg Config) *Client {
	c := NewClient()
	if cfg.BaseURL != "" {
		c.BaseURL = cfg.BaseURL
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c
}

// Artifact is one Maven artifact, as returned by a default-mode search
// (latest version per artifact).
type Artifact struct {
	ID            string   `kit:"id" json:"id"`
	GroupID       string   `json:"group_id"`
	ArtifactID    string   `json:"artifact_id"`
	LatestVersion string   `json:"latest_version"`
	Packaging     string   `json:"packaging"`
	VersionCount  int      `json:"version_count"`
	UpdatedAt     int64    `json:"updated_at_ms"`
	Extensions    []string `json:"extensions"`
}

// Version is one released version of a specific Maven artifact, as returned by
// a core=gav search.
type Version struct {
	ID         string `kit:"id" json:"id"`
	GroupID    string `json:"group_id"`
	ArtifactID string `json:"artifact_id"`
	Version    string `json:"version"`
	Packaging  string `json:"packaging"`
	UpdatedAt  int64  `json:"updated_at_ms"`
}

// --- wire types ---

type wireDoc struct {
	ID            string   `json:"id"`
	G             string   `json:"g"`
	A             string   `json:"a"`
	V             string   `json:"v"`
	LatestVersion string   `json:"latestVersion"`
	P             string   `json:"p"`
	Timestamp     int64    `json:"timestamp"`
	VersionCount  int      `json:"versionCount"`
	EC            []string `json:"ec"`
}

type wireResponse struct {
	Response struct {
		NumFound int       `json:"numFound"`
		Docs     []wireDoc `json:"docs"`
	} `json:"response"`
}

// Search searches Maven Central for artifacts matching query. It returns up to
// limit results, each showing the latest version of a matching artifact.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Artifact, error) {
	if limit <= 0 {
		limit = 20
	}
	u := fmt.Sprintf("%s/select?q=%s&rows=%d&wt=json",
		c.BaseURL, url.QueryEscape(query), limit)
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var wr wireResponse
	if err := json.Unmarshal(body, &wr); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}
	out := make([]Artifact, 0, len(wr.Response.Docs))
	for _, d := range wr.Response.Docs {
		out = append(out, Artifact{
			ID:            d.G + ":" + d.A,
			GroupID:       d.G,
			ArtifactID:    d.A,
			LatestVersion: d.LatestVersion,
			Packaging:     d.P,
			VersionCount:  d.VersionCount,
			UpdatedAt:     d.Timestamp,
			Extensions:    d.EC,
		})
	}
	return out, nil
}

// GetVersions lists all known versions of a specific artifact identified by
// groupID and artifactID. It returns up to limit results.
func (c *Client) GetVersions(ctx context.Context, groupID, artifactID string, limit int) ([]Version, error) {
	if limit <= 0 {
		limit = 20
	}
	q := fmt.Sprintf("g:%s+AND+a:%s", groupID, artifactID)
	u := fmt.Sprintf("%s/select?q=%s&core=gav&rows=%d&wt=json",
		c.BaseURL, q, limit)
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var wr wireResponse
	if err := json.Unmarshal(body, &wr); err != nil {
		return nil, fmt.Errorf("decode versions response: %w", err)
	}
	out := make([]Version, 0, len(wr.Response.Docs))
	for _, d := range wr.Response.Docs {
		g := d.G
		if g == "" {
			g = groupID
		}
		a := d.A
		if a == "" {
			a = artifactID
		}
		out = append(out, Version{
			ID:         g + ":" + a + ":" + d.V,
			GroupID:    g,
			ArtifactID: a,
			Version:    d.V,
			Packaging:  d.P,
			UpdatedAt:  d.Timestamp,
		})
	}
	return out, nil
}

// Get fetches url and returns the response body. It paces and retries according
// to the client's settings. The caller owns nothing extra; the body is read
// fully and closed here.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
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

func (c *Client) do(ctx context.Context, u string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
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

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
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
