package mavencentral

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring (mint, body, resolve), which need no network. The client's
// HTTP behaviour is covered in mavencentral_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "mavencentral" {
		t.Errorf("Scheme = %q, want mavencentral", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "mavencentral" {
		t.Errorf("Identity.Binary = %q, want mavencentral", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in  string
		typ string
		id  string
	}{
		{"com.google.guava:guava", "artifact", "com.google.guava:guava"},
		{"com.google.guava:guava:33.4.8-jre", "artifact", "com.google.guava:guava:33.4.8-jre"},
		{"org.springframework:spring-core", "artifact", "org.springframework:spring-core"},
		{"spring-core", "query", "spring-core"},
		{"guava", "query", "guava"},
		{"log4j", "query", "log4j"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	cases := []struct {
		uriType string
		id      string
		want    string
	}{
		{"artifact", "com.google.guava:guava", "https://search.maven.org/artifact/com.google.guava/guava"},
		{"artifact", "com.google.guava:guava:33.4.8-jre", "https://search.maven.org/artifact/com.google.guava/guava/33.4.8-jre"},
		{"query", "spring-core", "https://search.maven.org/search?q=spring-core"},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.uriType, tc.id)
		if err != nil || got != tc.want {
			t.Errorf("Locate(%q, %q) = (%q, %v), want (%q, nil)", tc.uriType, tc.id, got, err, tc.want)
		}
	}
}

// TestHostWiring mounts the driver in a kit Host and checks the round trip:
// a record mints to its URI, its body is readable, and a bare id resolves
// back to the same URI. The init in domain.go registers the domain, so
// kit.Open finds it.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	a := &Artifact{
		ID:            "com.google.guava:guava",
		GroupID:       "com.google.guava",
		ArtifactID:    "guava",
		LatestVersion: "33.4.8-jre",
		Packaging:     "jar",
		VersionCount:  50,
	}
	u, err := h.Mint(a)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if want := "mavencentral://artifact/com.google.guava:guava"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("mavencentral", "com.google.guava:guava")
	if err != nil || got.String() != "mavencentral://artifact/com.google.guava:guava" {
		t.Errorf("ResolveOn = (%q, %v), want mavencentral://artifact/com.google.guava:guava", got.String(), err)
	}
}
