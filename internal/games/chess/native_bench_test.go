package chess

import (
	"context"
	"testing"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
)

// Several positions so the pooled transposition table cannot make every
// iteration a warm hit.
var benchFENs = []string{
	"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1",
	"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1",
	"r1bqkbnr/pppp1ppp/2n5/4p3/2B1P3/5N2/PPPP1PPP/RNBQK2R b KQkq - 3 3",
	"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1",
}

func BenchmarkNativeSearch(b *testing.B) {
	g := New().WithNativeEngine()
	opts := game.SearchOptions{Difficulty: "normal", TimeLimit: 200 * time.Millisecond}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pos, err := g.Decode(game.PositionInput{FEN: benchFENs[i%len(benchFENs)]})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := g.Search(context.Background(), pos, opts); err != nil {
			b.Fatal(err)
		}
	}
}
