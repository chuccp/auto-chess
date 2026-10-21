package reversi

import (
	"context"
	"testing"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
)

func TestInitialMoves(t *testing.T) {
	g := &Game{}
	st := g.Initial().(*State)
	moves := g.LegalMoves(st)
	if len(moves) != 4 {
		t.Fatalf("initial legal moves = %d, want 4", len(moves))
	}
	want := map[int]bool{19: true, 26: true, 37: true, 44: true}
	for _, m := range moves {
		if m.Index == nil || !want[m.Index.Index] {
			t.Fatalf("unexpected initial move: %+v", m)
		}
	}
}

func TestApplyFlips(t *testing.T) {
	g := &Game{}
	st := g.Initial().(*State)
	pos, mv, err := g.Apply(st, game.MoveInput{Index: intPtr(19)})
	if err != nil {
		t.Fatal(err)
	}
	if mv.Flipped != 1 {
		t.Fatalf("d3 should flip 1 disc, got %d", mv.Flipped)
	}
	next := pos.(*State)
	if bit(next.Black, 27) == 0 || bit(next.White, 27) != 0 {
		t.Fatal("d4 was not flipped to black")
	}
	if next.Turn != White {
		t.Fatal("turn should pass to white")
	}
}

func TestSearchReturnsLegal(t *testing.T) {
	g := &Game{}
	res, err := g.Search(context.Background(), g.Initial(), game.SearchOptions{
		Difficulty: "easy", MaxDepth: 2, TimeLimit: time.Second, Seed: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Move == nil || res.Move.Index == nil {
		t.Fatal("search returned no move")
	}
	if _, _, err := g.Apply(g.Initial(), game.MoveInput{Index: intPtr(res.Move.Index.Index)}); err != nil {
		t.Fatalf("search returned illegal move: %v", err)
	}
}

func TestGameOverByDiscCount(t *testing.T) {
	g := &Game{}
	st := &State{Black: ^uint64(0), Turn: Black, Last: -1}
	view := g.Encode(st, false)
	if view.Status.State != "win" || view.Status.Winner != "b" {
		t.Fatalf("full black board status = %+v", view.Status)
	}
}

func bit(board uint64, idx int) uint64 { return board & (1 << uint(idx)) }

func intPtr(v int) *int { return &v }
