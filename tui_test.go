package main

import (
	"strings"
	"testing"
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
