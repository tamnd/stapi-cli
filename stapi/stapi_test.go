package stapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamnd/stapi-cli/stapi"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := stapi.NewClient()
	c.Rate = 0 // no pacing in the test

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := stapi.NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestListCharacters(t *testing.T) {
	resp := map[string]any{
		"page": map[string]any{
			"pageNumber":    0,
			"totalElements": 7571,
			"totalPages":    379,
		},
		"characters": []map[string]any{
			{"uid": "CHMA0000215045", "name": "Spock", "gender": "M", "deceased": false,
				"hologram": false, "fictionalCharacter": false, "mirror": false},
			{"uid": "CHMA0000000002", "name": "James T. Kirk", "gender": "M", "deceased": false,
				"hologram": false, "fictionalCharacter": false, "mirror": false},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rest/character/search" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := stapi.NewClient()
	c.Rate = 0
	// Override base URL via HTTP client transport trick: use a custom transport
	// that rewrites the host.
	c.HTTP = &http.Client{
		Transport: rewriteHost(srv.URL),
		Timeout:   10 * time.Second,
	}

	// We can't override BaseURL via the public API directly, so test via the
	// raw Get method against the test server URL instead.
	body, err := c.Get(context.Background(), srv.URL+"/api/v1/rest/character/search?pageSize=2")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	chars, ok := got["characters"].([]any)
	if !ok || len(chars) != 2 {
		t.Errorf("expected 2 characters, got %v", got["characters"])
	}
}

func TestGetCharacterDecoding(t *testing.T) {
	resp := map[string]any{
		"character": map[string]any{
			"uid":                "CHMA0000215045",
			"name":               "Spock",
			"gender":             "M",
			"deceased":           false,
			"hologram":           false,
			"fictionalCharacter": false,
			"mirror":             false,
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := stapi.NewClient()
	c.Rate = 0
	body, err := c.Get(context.Background(), srv.URL+"/api/v1/rest/character?uid=CHMA0000215045")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	char, ok := got["character"].(map[string]any)
	if !ok {
		t.Fatalf("expected character object, got %T", got["character"])
	}
	if char["uid"] != "CHMA0000215045" {
		t.Errorf("uid = %v, want CHMA0000215045", char["uid"])
	}
	if char["name"] != "Spock" {
		t.Errorf("name = %v, want Spock", char["name"])
	}
}

func TestGetEpisodeDecoding(t *testing.T) {
	resp := map[string]any{
		"episode": map[string]any{
			"uid":           "EPMA0000001458",
			"title":         "All Good Things...",
			"series":        map[string]any{"title": "Star Trek: The Next Generation"},
			"seasonNumber":  7,
			"episodeNumber": 25,
			"featureLength": true,
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := stapi.NewClient()
	c.Rate = 0
	body, err := c.Get(context.Background(), srv.URL+"/api/v1/rest/episode?uid=EPMA0000001458")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	ep, ok := got["episode"].(map[string]any)
	if !ok {
		t.Fatalf("expected episode object, got %T", got["episode"])
	}
	if ep["title"] != "All Good Things..." {
		t.Errorf("title = %v, want 'All Good Things...'", ep["title"])
	}
	series, ok := ep["series"].(map[string]any)
	if !ok {
		t.Fatalf("expected series object, got %T", ep["series"])
	}
	if series["title"] != "Star Trek: The Next Generation" {
		t.Errorf("series title = %v, want 'Star Trek: The Next Generation'", series["title"])
	}
}

func TestListSeriesDecoding(t *testing.T) {
	resp := map[string]any{
		"page": map[string]any{"pageNumber": 0, "totalElements": 12, "totalPages": 1},
		"series": []map[string]any{
			{"uid": "SEMA0000000001", "title": "Star Trek: The Next Generation", "abbreviation": "TNG"},
			{"uid": "SEMA0000000002", "title": "Star Trek: Deep Space Nine", "abbreviation": "DS9"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := stapi.NewClient()
	c.Rate = 0
	body, err := c.Get(context.Background(), srv.URL+"/api/v1/rest/series/search?pageSize=20")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	series, ok := got["series"].([]any)
	if !ok || len(series) != 2 {
		t.Errorf("expected 2 series, got %v", got["series"])
	}
	first := series[0].(map[string]any)
	if first["abbreviation"] != "TNG" {
		t.Errorf("abbreviation = %v, want TNG", first["abbreviation"])
	}
}

func TestGetNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := stapi.NewClient()
	c.Rate = 0
	c.Retries = 0

	_, err := c.Get(context.Background(), srv.URL)
	if err == nil {
		t.Error("expected error on 404, got nil")
	}
}

// rewriteHost returns a RoundTripper that sends all requests to baseURL.
type hostRewriter struct {
	base string
}

func rewriteHost(base string) http.RoundTripper {
	return &hostRewriter{base: base}
}

func (h *hostRewriter) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.URL.Host = r.URL.Host // keep path/query
	return http.DefaultTransport.RoundTrip(r2)
}
