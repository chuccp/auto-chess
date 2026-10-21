package xiangqi

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
)

func BenchmarkSearchNNUE(b *testing.B) {
	path := os.Getenv("PIKAFISH_NNUE")
	if path == "" {
		b.Skip("set PIKAFISH_NNUE")
	}
	g, err := NewWithNNUE(path)
	if err != nil {
		b.Fatal(err)
	}
	pos := g.Initial()
	opts := game.SearchOptions{
		Difficulty: "normal",
		TimeLimit:  300 * time.Millisecond,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := g.Search(context.Background(), pos, opts); err != nil {
			b.Fatal(err)
		}
	}
}
