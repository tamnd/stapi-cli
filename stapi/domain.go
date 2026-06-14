package stapi

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes the Star Trek API (stapi.co) as a kit Domain: a driver
// that a multi-domain host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/stapi-cli/stapi"
//
// The init below registers it; the host then dereferences stapi:// URIs by
// routing to the operations Register installs. The same Domain also builds
// the standalone stapi binary (see cli.NewApp), so the binary and a host
// share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the stapi driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "stapi",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "stapi",
			Short:  "A command line for the Star Trek API.",
			Long: `A command line for the Star Trek API (stapi.co).

stapi reads public Star Trek data over plain HTTPS, shapes it into
clean records, and prints output that pipes into the rest of your tools. No API
key, nothing to run alongside it.`,
			Site: Host,
			Repo: "https://github.com/tamnd/stapi-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "characters", Group: "read", List: true,
		Summary: "List Star Trek characters"}, listCharacters)

	kit.Handle(app, kit.OpMeta{Name: "character", Group: "read", Single: true,
		Summary: "Fetch a character by UID", URIType: "character", Resolver: true,
		Args: []kit.Arg{{Name: "uid", Help: "character UID (e.g. CHMA0000215045)"}}}, getCharacter)

	kit.Handle(app, kit.OpMeta{Name: "episodes", Group: "read", List: true,
		Summary: "List Star Trek episodes"}, listEpisodes)

	kit.Handle(app, kit.OpMeta{Name: "episode", Group: "read", Single: true,
		Summary: "Fetch an episode by UID", URIType: "episode", Resolver: true,
		Args: []kit.Arg{{Name: "uid", Help: "episode UID (e.g. EPMA0000001458)"}}}, getEpisode)

	kit.Handle(app, kit.OpMeta{Name: "series", Group: "read", List: true,
		Summary: "List all Star Trek series"}, listSeries)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
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
	return c, nil
}

// --- inputs ---

type charactersInput struct {
	Limit  int     `kit:"flag,inherit" help:"max characters" default:"20"`
	Client *Client `kit:"inject"`
}

type characterInput struct {
	UID    string  `kit:"arg" help:"character UID (e.g. CHMA0000215045)"`
	Client *Client `kit:"inject"`
}

type episodesInput struct {
	Limit  int     `kit:"flag,inherit" help:"max episodes" default:"20"`
	Client *Client `kit:"inject"`
}

type episodeInput struct {
	UID    string  `kit:"arg" help:"episode UID (e.g. EPMA0000001458)"`
	Client *Client `kit:"inject"`
}

type seriesInput struct {
	Client *Client `kit:"inject"`
}

// --- handlers ---

func listCharacters(ctx context.Context, in charactersInput, emit func(Character) error) error {
	chars, err := in.Client.ListCharacters(ctx, in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for _, ch := range chars {
		if err := emit(ch); err != nil {
			return err
		}
	}
	return nil
}

func getCharacter(ctx context.Context, in characterInput, emit func(*Character) error) error {
	ch, err := in.Client.GetCharacter(ctx, in.UID)
	if err != nil {
		return mapErr(err)
	}
	return emit(ch)
}

func listEpisodes(ctx context.Context, in episodesInput, emit func(Episode) error) error {
	eps, err := in.Client.ListEpisodes(ctx, in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for _, ep := range eps {
		if err := emit(ep); err != nil {
			return err
		}
	}
	return nil
}

func getEpisode(ctx context.Context, in episodeInput, emit func(*Episode) error) error {
	ep, err := in.Client.GetEpisode(ctx, in.UID)
	if err != nil {
		return mapErr(err)
	}
	return emit(ep)
}

func listSeries(ctx context.Context, in seriesInput, emit func(Series) error) error {
	all, err := in.Client.ListSeries(ctx)
	if err != nil {
		return mapErr(err)
	}
	for _, s := range all {
		if err := emit(s); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: URI driver string functions, pure and network-free ---

// Classify turns any accepted input into the canonical (type, id).
// UID pattern detection: "CH" prefix -> character, "EP" -> episode, "SE" -> series.
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("empty stapi reference")
	}
	upper := strings.ToUpper(input)
	switch {
	case strings.HasPrefix(upper, "CH"):
		return "character", input, nil
	case strings.HasPrefix(upper, "EP"):
		return "episode", input, nil
	case strings.HasPrefix(upper, "SE"):
		return "series", input, nil
	default:
		return "query", input, nil
	}
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "character":
		return "https://www.star-trek.com/character/" + id, nil
	case "episode":
		return BaseURL + "/api/v1/rest/episode?uid=" + id, nil
	case "series":
		return BaseURL + "/api/v1/rest/series?uid=" + id, nil
	default:
		return "", errs.Usage("stapi has no resource type %q", uriType)
	}
}

// --- helpers ---

// mapErr converts a library error into the kit error kind that carries the
// right exit code.
func mapErr(err error) error {
	return err
}
