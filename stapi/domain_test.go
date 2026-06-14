package stapi

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string
// functions and the host wiring (mint, body, resolve), which need no network.
// The client's HTTP behaviour is covered in stapi_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "stapi" {
		t.Errorf("Scheme = %q, want stapi", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "stapi" {
		t.Errorf("Identity.Binary = %q, want stapi", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in  string
		typ string
		id  string
	}{
		{"CHMA0000215045", "character", "CHMA0000215045"},
		{"EPMA0000001458", "episode", "EPMA0000001458"},
		{"SEMA0000000001", "series", "SEMA0000000001"},
		{"something-else", "query", "something-else"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil {
			t.Errorf("Classify(%q) returned error: %v", tc.in, err)
			continue
		}
		if typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q), want (%q, %q)",
				tc.in, typ, id, tc.typ, tc.id)
		}
	}
}

func TestClassifyEmpty(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("Classify(\"\") should return an error")
	}
}

func TestLocate(t *testing.T) {
	cases := []struct {
		typ  string
		id   string
		want string
	}{
		{"character", "CHMA0000215045", "https://www.star-trek.com/character/CHMA0000215045"},
		{"episode", "EPMA0000001458", BaseURL + "/api/v1/rest/episode?uid=EPMA0000001458"},
		{"series", "SEMA0000000001", BaseURL + "/api/v1/rest/series?uid=SEMA0000000001"},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.typ, tc.id)
		if err != nil || got != tc.want {
			t.Errorf("Locate(%q, %q) = (%q, %v), want (%q, nil)",
				tc.typ, tc.id, got, err, tc.want)
		}
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "X")
	if err == nil {
		t.Error("Locate with unknown type should return an error")
	}
}

func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	ch := &Character{UID: "CHMA0000215045", Name: "Spock"}
	u, err := h.Mint(ch)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if want := "stapi://character/CHMA0000215045"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("stapi", "EPMA0000001458")
	if err != nil || got.String() != "stapi://episode/EPMA0000001458" {
		t.Errorf("ResolveOn = (%q, %v), want stapi://episode/EPMA0000001458", got.String(), err)
	}
}

func TestFromWireCharacter(t *testing.T) {
	w := wireCharacter{
		UID:                "CHMA0000215045",
		Name:               "Spock",
		Gender:             "M",
		Deceased:           false,
		Hologram:           false,
		FictionalCharacter: false,
		Mirror:             false,
	}
	ch := fromWireCharacter(w)
	if ch.UID != w.UID {
		t.Errorf("UID = %q, want %q", ch.UID, w.UID)
	}
	if ch.Name != w.Name {
		t.Errorf("Name = %q, want %q", ch.Name, w.Name)
	}
	if ch.Gender != w.Gender {
		t.Errorf("Gender = %q, want %q", ch.Gender, w.Gender)
	}
}

func TestFromWireEpisode(t *testing.T) {
	w := wireEpisode{
		UID:           "EPMA0000001458",
		Title:         "All Good Things...",
		SeasonNumber:  7,
		EpisodeNumber: 25,
		FeatureLength: true,
	}
	w.Series.Title = "Star Trek: The Next Generation"
	ep := fromWireEpisode(w)
	if ep.UID != w.UID {
		t.Errorf("UID = %q, want %q", ep.UID, w.UID)
	}
	if ep.Series != "Star Trek: The Next Generation" {
		t.Errorf("Series = %q, want %q", ep.Series, "Star Trek: The Next Generation")
	}
	if ep.SeasonNumber != 7 {
		t.Errorf("SeasonNumber = %d, want 7", ep.SeasonNumber)
	}
	if !ep.FeatureLength {
		t.Error("FeatureLength should be true")
	}
}
