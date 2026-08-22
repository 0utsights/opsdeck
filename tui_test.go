package main

import (
	"strings"
	"testing"
	"time"
)

func TestBarHasStableWidth(t *testing.T) {
	got := bar(50, 10)
	if !strings.HasPrefix(got, "█████─────") || !strings.HasSuffix(got, "  50%") {
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

func TestPageRegistryAndRotation(t *testing.T) {
	if len(pageDefinitions) != 2 || pageDefinitions[0].name != "OVERVIEW" || pageDefinitions[1].name != "FLEET" {
		t.Fatalf("unexpected page registry: %#v", pageDefinitions)
	}
	start := time.Unix(1_700_000_000, 0)
	a := &app{cfg: Config{PageSeconds: 20}, pageChangedAt: start}
	if a.maybeRotatePage(start.Add(19 * time.Second)) {
		t.Fatal("page rotated before interval")
	}
	if !a.maybeRotatePage(start.Add(20*time.Second)) || a.pageIndex != 1 || a.focus != 1 {
		t.Fatalf("page did not rotate into fleet: page=%d focus=%d", a.pageIndex, a.focus)
	}
	a.pagePaused = true
	if a.maybeRotatePage(start.Add(40 * time.Second)) {
		t.Fatal("paused page rotation advanced")
	}
	a.setPage(-1, start)
	if a.pageIndex != len(pageDefinitions)-1 {
		t.Fatalf("negative page index did not wrap: %d", a.pageIndex)
	}
}
