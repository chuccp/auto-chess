package checkers

import (
	"context"
	"testing"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
)

func BenchmarkGenMoves(b *testing.B) {
	st := (&Game{}).Initial().(*State)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		genMoves(st)
	}
}

func BenchmarkSearchMaster(b *testing.B) {
	g := New()
	pos := g.Initial()
	opts := game.SearchOptions{Difficulty: "master", TimeLimit: 5 * time.Second}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := g.Search(context.Background(), pos, opts); err != nil {
			b.Fatal(err)
		}
	}
}
