package chess

import (
	"context"
	"testing"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
)

func TestNativeEngineSearch(t *testing.T) {
	g := New().WithNativeEngine()
	start := time.Now()
	res, err := g.Search(context.Background(), g.Initial(), game.SearchOptions{
		Difficulty: "easy",
		MaxDepth:   6,
		TimeLimit:  500 * time.Millisecond,
		Seed:       1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Engine != "chess-3" {
		t.Fatalf("engine = %q, want chess-3", res.Engine)
	}
	if res.Move == nil || res.Move.UCI == "" {
		t.Fatalf("no native move: %+v", res)
	}
	if res.Search.Depth < 1 || res.Search.Nodes == 0 {
		t.Fatalf("missing native search stats: %+v", res.Search)
	}
	if res.Move.UCI == "0000" {
		t.Fatal("native engine returned a null move")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("native search took too long: %v", elapsed)
	}
}
