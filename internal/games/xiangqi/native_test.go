package xiangqi

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
		MaxDepth:   3,
		TimeLimit:  500 * time.Millisecond,
		Seed:       1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Engine != "godogpaw" {
		t.Fatalf("engine = %q, want godogpaw", res.Engine)
	}
	if res.Move == nil || res.Move.UCI == "" {
		t.Fatalf("no native move: %+v", res)
	}
	if res.Search.Depth < 1 || res.Search.Nodes == 0 {
		t.Fatalf("missing native search stats: %+v", res.Search)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("native search took too long: %v", elapsed)
	}
}
