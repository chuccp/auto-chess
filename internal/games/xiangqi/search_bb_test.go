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

func TestTTMateScorePlyAdjustment(t *testing.T) {
	for _, ply := range []int{0, 1, 5, 12} {
		for _, score := range []int{mateScore - 3, -mateScore + 3, 0, 250, -250} {
			got := ttScoreFromPly(ttScore(score, ply), ply)
			if got != score {
				t.Fatalf("ply %d score %d round trip = %d", ply, score, got)
			}
		}
	}
	// mateScore-5 at ply 4 is "mate in 1 from this node". Reached again at
	// ply 2 the root distance shrinks by 2, so the probed score must be 2 plies
	// closer to mate even though the stored entry is the same.
	stored := ttScore(mateScore-5, 4)
	if got := ttScoreFromPly(stored, 2); got != mateScore-3 {
		t.Fatalf("mate distance not adjusted: got %d, want %d", got, mateScore-3)
	}
}
