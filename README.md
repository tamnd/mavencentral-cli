# mavencentral

A command line for Maven Central.

`mavencentral` is a single pure-Go binary. It reads public mavencentral data
over plain HTTPS, shapes it into clean records, and prints output that pipes
into the rest of your tools. No API key, nothing to run alongside it.

The same package is also a [resource-URI driver](#use-it-as-a-resource-uri-driver),
so a host program like [ant](https://github.com/tamnd/ant) can address
mavencentral as `mavencentral://` URIs.

## Install

```bash
go install github.com/tamnd/mavencentral-cli/cmd/mavencentral@latest
```

Or grab a prebuilt binary from the [releases](https://github.com/tamnd/mavencentral-cli/releases), or run
the container image:

```bash
docker run --rm ghcr.io/tamnd/mavencentral:latest --help
```

## Usage

```bash
# Search for artifacts by keyword
mavencentral search spring-core
mavencentral search guava -o json
mavencentral search log4j -n 5

# List all versions of a specific artifact
mavencentral versions com.google.guava guava
mavencentral versions org.springframework spring-core -n 10 -o json

mavencentral --help                           # the whole command tree
```

Every command shares one output contract:
`-o table|markdown|json|jsonl|csv|tsv|url|raw`, `--fields` to pick columns,
`--template` for a custom line, and `-n` to limit. The default adapts to where
output goes (a color-aware table on a terminal, JSONL in a pipe), so the same
command reads well by hand and parses cleanly downstream.

## Serve it

The same operations are available over HTTP and as an MCP tool set for agents,
with no extra code:

```bash
mavencentral serve --addr :7777    # GET /v1/page/<path>  returns NDJSON
mavencentral mcp                   # speak MCP over stdio
```

## Use it as a resource-URI driver

`mavencentral` registers a `mavencentral` domain the way a program registers a
database driver with `database/sql`. A host enables it with one blank import:

```go
import _ "github.com/tamnd/mavencentral-cli/mavencentral"
```

Then [ant](https://github.com/tamnd/ant) (or any program that links the package)
dereferences `mavencentral://` URIs without knowing anything about mavencentral:

```bash
ant get mavencentral://artifact/com.google.guava:guava   # resolve the URL
ant url mavencentral://artifact/com.google.guava:guava   # the live https URL
```

## Development

```
cmd/mavencentral/   thin main: hands cli.NewApp to kit.Run
cli/                 assembles the kit App from the mavencentral domain
mavencentral/                the library: HTTP client, data models, and domain.go (the driver)
docs/                tago documentation site
```

```bash
make build      # ./bin/mavencentral
make test       # go test ./...
make vet        # go vet ./...
```

## Releasing

Push a version tag and GitHub Actions runs GoReleaser, which builds the
archives, Linux packages, the multi-arch GHCR image, checksums, SBOMs, and a
cosign signature:

```bash
git tag v0.1.0
git push --tags
```

The Homebrew and Scoop steps self-disable until their tokens exist, so the first
release works with no extra secrets.

## License

Apache-2.0. See [LICENSE](LICENSE).
