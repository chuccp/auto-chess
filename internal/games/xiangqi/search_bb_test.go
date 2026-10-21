package xiangqi

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
)

func TestPositionSearchNNUE(t *testing.T) {
	path := os.Getenv("PIKAFISH_NNUE")
	if path == "" {
		t.Skip("set PIKAFISH_NNUE")
	}
	g, err := NewWithNNUE(path)
	if err != nil {
		t.Fatal(err)
	}
	pos := g.Initial()
	res, err := g.Search(context.Background(), pos, game.SearchOptions{Difficulty: "normal", TimeLimit: 500 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if res.Move == nil {
		t.Fatal("no move")
	}
	found := false
	for _, m := range legalMoves(pos.(*State)) {
		if m.uci() == res.Move.UCI {
			found = true
		}
	}
	if !found {
		t.Fatalf("search returned illegal move %s", res.Move.UCI)
	}
	t.Logf("move=%s depth=%d nodes=%d", res.Move.UCI, res.Search.Depth, res.Search.Nodes)
}
