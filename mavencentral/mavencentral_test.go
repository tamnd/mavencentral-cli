package mavencentral

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
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

	c := NewClient()
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

func TestSearch(t *testing.T) {
	resp := wireResponse{}
	resp.Response.NumFound = 1
	resp.Response.Docs = []wireDoc{
		{
			ID:            "com.google.guava:guava",
			G:             "com.google.guava",
			A:             "guava",
			LatestVersion: "33.4.8-jre",
			P:             "jar",
			Timestamp:     1744651522000,
			VersionCount:  50,
			EC:            []string{"-javadoc.jar", "-sources.jar", ".jar", ".pom"},
		},
	}
	b, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") == "" {
			t.Error("search request missing q param")
		}
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.BaseURL = srv.URL

	results, err := c.Search(context.Background(), "guava", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	got := results[0]
	if got.GroupID != "com.google.guava" {
		t.Errorf("GroupID = %q, want com.google.guava", got.GroupID)
	}
	if got.ArtifactID != "guava" {
		t.Errorf("ArtifactID = %q, want guava", got.ArtifactID)
	}
	if got.LatestVersion != "33.4.8-jre" {
		t.Errorf("LatestVersion = %q, want 33.4.8-jre", got.LatestVersion)
	}
	if got.ID != "com.google.guava:guava" {
		t.Errorf("ID = %q, want com.google.guava:guava", got.ID)
	}
	if got.VersionCount != 50 {
		t.Errorf("VersionCount = %d, want 50", got.VersionCount)
	}
}

func TestGetVersions(t *testing.T) {
	resp := wireResponse{}
	resp.Response.NumFound = 2
	resp.Response.Docs = []wireDoc{
		{G: "com.google.guava", A: "guava", V: "33.4.8-jre", P: "jar", Timestamp: 1744651522000},
		{G: "com.google.guava", A: "guava", V: "33.4.7-jre", P: "jar", Timestamp: 1740000000000},
	}
	b, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			t.Error("versions request missing q param")
		}
		if r.URL.Query().Get("core") != "gav" {
			t.Error("versions request missing core=gav")
		}
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.BaseURL = srv.URL

	versions, err := c.GetVersions(context.Background(), "com.google.guava", "guava", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("got %d versions, want 2", len(versions))
	}
	v := versions[0]
	if v.Version != "33.4.8-jre" {
		t.Errorf("Version = %q, want 33.4.8-jre", v.Version)
	}
	if v.ID != "com.google.guava:guava:33.4.8-jre" {
		t.Errorf("ID = %q, want com.google.guava:guava:33.4.8-jre", v.ID)
	}
	if v.GroupID != "com.google.guava" {
		t.Errorf("GroupID = %q, want com.google.guava", v.GroupID)
	}
}
