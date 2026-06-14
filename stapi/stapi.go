// Package stapi is the library behind the stapi command line:
// the HTTP client, request shaping, and the typed data models for the
// Star Trek API (stapi.co).
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public site throws under load.
package stapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultUserAgent identifies the client to stapi.co.
const DefaultUserAgent = "stapi-cli/dev (+https://github.com/tamnd/stapi-cli)"

// Host is the site this client talks to.
const Host = "stapi.co"

// BaseURL is the root every request is built from.
const BaseURL = "https://" + Host

// Client talks to stapi.co over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults: a 30s timeout, a 300ms
// minimum gap between requests, and five retries on transient errors.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		UserAgent: DefaultUserAgent,
		Rate:      300 * time.Millisecond,
		Retries:   5,
	}
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

func (c *Client) do(ctx context.Context, url string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")

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

// --- output types ---

// Character is a Star Trek character.
type Character struct {
	UID                string `kit:"id" json:"uid"`
	Name               string `json:"name"`
	Gender             string `json:"gender"`
	Deceased           bool   `json:"deceased"`
	Hologram           bool   `json:"hologram"`
	FictionalCharacter bool   `json:"fictionalCharacter"`
	Mirror             bool   `json:"mirror"`
}

// Episode is a Star Trek episode.
type Episode struct {
	UID           string `kit:"id" json:"uid"`
	Title         string `json:"title"`
	Series        string `json:"series"` // extracted from series.title
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeNumber int    `json:"episodeNumber"`
	FeatureLength bool   `json:"featureLength"`
}

// Series is a Star Trek series.
type Series struct {
	UID          string `kit:"id" json:"uid"`
	Title        string `json:"title"`
	Abbreviation string `json:"abbreviation"`
}

// --- wire types ---

type pageInfo struct {
	PageNumber    int `json:"pageNumber"`
	TotalElements int `json:"totalElements"`
	TotalPages    int `json:"totalPages"`
}

type wireCharacter struct {
	UID                string `json:"uid"`
	Name               string `json:"name"`
	Gender             string `json:"gender"`
	Deceased           bool   `json:"deceased"`
	Hologram           bool   `json:"hologram"`
	FictionalCharacter bool   `json:"fictionalCharacter"`
	Mirror             bool   `json:"mirror"`
}

type wireEpisode struct {
	UID    string `json:"uid"`
	Title  string `json:"title"`
	Series struct {
		Title string `json:"title"`
	} `json:"series"`
	SeasonNumber  int  `json:"seasonNumber"`
	EpisodeNumber int  `json:"episodeNumber"`
	FeatureLength bool `json:"featureLength"`
}

type wireSeries struct {
	UID          string `json:"uid"`
	Title        string `json:"title"`
	Abbreviation string `json:"abbreviation"`
}

type charSearchResponse struct {
	Page       pageInfo        `json:"page"`
	Characters []wireCharacter `json:"characters"`
}

type episodeSearchResponse struct {
	Page     pageInfo      `json:"page"`
	Episodes []wireEpisode `json:"episodes"`
}

type seriesSearchResponse struct {
	Page   pageInfo     `json:"page"`
	Series []wireSeries `json:"series"`
}

type charResponse struct {
	Character wireCharacter `json:"character"`
}

type episodeResponse struct {
	Episode wireEpisode `json:"episode"`
}

// --- conversion helpers ---

func fromWireCharacter(w wireCharacter) Character {
	return Character{
		UID:                w.UID,
		Name:               w.Name,
		Gender:             w.Gender,
		Deceased:           w.Deceased,
		Hologram:           w.Hologram,
		FictionalCharacter: w.FictionalCharacter,
		Mirror:             w.Mirror,
	}
}

func fromWireEpisode(w wireEpisode) Episode {
	return Episode{
		UID:           w.UID,
		Title:         w.Title,
		Series:        w.Series.Title,
		SeasonNumber:  w.SeasonNumber,
		EpisodeNumber: w.EpisodeNumber,
		FeatureLength: w.FeatureLength,
	}
}

// --- client methods ---

// ListCharacters fetches a page of characters from the search endpoint.
func (c *Client) ListCharacters(ctx context.Context, limit int) ([]Character, error) {
	url := fmt.Sprintf("%s/api/v1/rest/character/search?pageSize=%d", BaseURL, limit)
	b, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp charSearchResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("decode characters: %w", err)
	}
	out := make([]Character, 0, len(resp.Characters))
	for _, w := range resp.Characters {
		out = append(out, fromWireCharacter(w))
	}
	return out, nil
}

// GetCharacter fetches a single character by UID.
func (c *Client) GetCharacter(ctx context.Context, uid string) (*Character, error) {
	url := fmt.Sprintf("%s/api/v1/rest/character?uid=%s", BaseURL, uid)
	b, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp charResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("decode character: %w", err)
	}
	ch := fromWireCharacter(resp.Character)
	return &ch, nil
}

// ListEpisodes fetches a page of episodes from the search endpoint.
func (c *Client) ListEpisodes(ctx context.Context, limit int) ([]Episode, error) {
	url := fmt.Sprintf("%s/api/v1/rest/episode/search?pageSize=%d", BaseURL, limit)
	b, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp episodeSearchResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("decode episodes: %w", err)
	}
	out := make([]Episode, 0, len(resp.Episodes))
	for _, w := range resp.Episodes {
		out = append(out, fromWireEpisode(w))
	}
	return out, nil
}

// GetEpisode fetches a single episode by UID.
func (c *Client) GetEpisode(ctx context.Context, uid string) (*Episode, error) {
	url := fmt.Sprintf("%s/api/v1/rest/episode?uid=%s", BaseURL, uid)
	b, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp episodeResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("decode episode: %w", err)
	}
	ep := fromWireEpisode(resp.Episode)
	return &ep, nil
}

// ListSeries fetches all Star Trek series.
func (c *Client) ListSeries(ctx context.Context) ([]Series, error) {
	url := fmt.Sprintf("%s/api/v1/rest/series/search?pageSize=20", BaseURL)
	b, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp seriesSearchResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("decode series: %w", err)
	}
	out := make([]Series, 0, len(resp.Series))
	for _, w := range resp.Series {
		out = append(out, Series{
			UID:          w.UID,
			Title:        w.Title,
			Abbreviation: w.Abbreviation,
		})
	}
	return out, nil
}
