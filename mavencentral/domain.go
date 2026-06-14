package mavencentral

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes Maven Central as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/mavencentral-cli/mavencentral"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// mavencentral:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone mavencentral binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the Maven Central driver. It carries no state; the per-run client
// is built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "mavencentral",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "mavencentral",
			Short:  "A command line for Maven Central.",
			Long: `A command line for Maven Central.

mavencentral reads public Maven Central data over plain HTTPS, shapes it into
clean records, and prints output that pipes into the rest of your tools. No API
key, nothing to run alongside it.`,
			Site: Host,
			Repo: "https://github.com/tamnd/mavencentral-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// search: find artifacts by keyword, returns latest version per artifact.
	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read",
		Summary: "Search Maven Central for artifacts",
		Args:    []kit.Arg{{Name: "query", Help: "search query (e.g. spring-core, guava, log4j)"}}}, searchArtifacts)

	// versions: list all known versions of a specific artifact.
	kit.Handle(app, kit.OpMeta{Name: "versions", Group: "read",
		Summary: "List versions of a Maven artifact",
		Args: []kit.Arg{
			{Name: "group", Help: "Maven group ID (e.g. com.google.guava)"},
			{Name: "artifact", Help: "Maven artifact ID (e.g. guava)"},
		}}, listVersions)
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

type searchInput struct {
	Query  string  `kit:"arg" help:"search query (e.g. spring-core, guava, log4j)"`
	Limit  int     `kit:"flag,inherit" help:"max results" default:"20"`
	Client *Client `kit:"inject"`
}

type versionsInput struct {
	GroupID    string  `kit:"arg" help:"Maven group ID (e.g. com.google.guava)"`
	ArtifactID string  `kit:"arg" help:"Maven artifact ID (e.g. guava)"`
	Limit      int     `kit:"flag,inherit" help:"max versions" default:"20"`
	Client     *Client `kit:"inject"`
}

// --- handlers ---

func searchArtifacts(ctx context.Context, in searchInput, emit func(Artifact) error) error {
	results, err := in.Client.Search(ctx, in.Query, in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for _, a := range results {
		if err := emit(a); err != nil {
			return err
		}
	}
	return nil
}

func listVersions(ctx context.Context, in versionsInput, emit func(Version) error) error {
	versions, err := in.Client.GetVersions(ctx, in.GroupID, in.ArtifactID, in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for _, v := range versions {
		if err := emit(v); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: the URI-native string functions, pure and network-free ---

// Classify turns any accepted input into the canonical (type, id).
// A "group:artifact:version" or "group:artifact" pattern (contains ":") is an
// artifact; anything else is treated as a query string.
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("empty Maven Central reference")
	}
	if strings.Contains(input, ":") {
		return "artifact", input, nil
	}
	return "query", input, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "artifact":
		// "group:artifact:version" or "group:artifact" -> artifact page
		return "https://search.maven.org/artifact/" + strings.ReplaceAll(id, ":", "/"), nil
	case "query":
		return "https://search.maven.org/search?q=" + id, nil
	default:
		return "", errs.Usage("mavencentral has no resource type %q", uriType)
	}
}

// mapErr converts a library error into the kit error kind that carries the
// right exit code.
func mapErr(err error) error {
	return err
}
