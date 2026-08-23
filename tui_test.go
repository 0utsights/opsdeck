package main

import (
	"strings"
	"testing"
)

func TestBarHasStableWidth(t *testing.T) {
	got := bar(50, 10)
	if !strings.HasPrefix(got, "━━━━━─────") || !strings.HasSuffix(got, "  50%") {
		t.Fatalf("unexpected bar: %q", got)
	}
}

func TestSparkClampsAndPads(t *testing.T) {
	got := spark([]float64{-1, 50, 101}, 5)
	if len([]rune(got)) != 5 {
		t.Fatalf("spark width = %d, want 5", len([]rune(got)))
	}
	if !strings.HasSuffix(got, " =@") {
		t.Fatalf("unexpected spark: %q", got)
	}
}

func TestTruncateUsesEllipsis(t *testing.T) {
	if got := truncate("abcdef", 4); got != "abc…" {
		t.Fatalf("truncate = %q", got)
	}
}

func TestSitePlacementAndMigrationStatus(t *testing.T) {
	site := SiteConfig{ID: "site", Name: "SITE", ContainerPatterns: []string{"site-web", "site-api"}}
	a := &app{
		cfg:   Config{Sites: []SiteConfig{site}},
		sites: []SiteState{{Config: site, Online: true}},
		servers: []ServerState{
			{Config: ServerConfig{ID: "source"}, Online: true, Probe: Probe{Containers: []ContainerInfo{{Name: "site-web-1"}}}},
			{Config: ServerConfig{ID: "target"}, Online: true, Probe: Probe{Containers: []ContainerInfo{{Name: "site-api"}}}},
		},
	}
	if got := a.sitesOnServer(a.servers[0]); len(got) != 1 || got[0].containers != 1 {
		t.Fatalf("unexpected hosted sites: %#v", got)
	}
	status, _ := a.migrationStatus(Migration{SiteID: "site", FromServer: "source", ToServer: "target"})
	if status != "MIRRORED" {
		t.Fatalf("migration status = %q, want MIRRORED", status)
	}
	a.servers[0].Probe.Containers = nil
	status, _ = a.migrationStatus(Migration{SiteID: "site", FromServer: "source", ToServer: "target"})
	if status != "MOVED" {
		t.Fatalf("migration status = %q, want MOVED", status)
	}
}
