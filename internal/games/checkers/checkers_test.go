package checkers

import (
	"context"
	"testing"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
)

func TestInitial(t *testing.T) {
	g := &Game{}
	view := g.Encode(g.Initial(), true)
	red, black := 0, 0
	for _, cell := range view.Board {
		switch cell {
		case "r":
			red++
		case "b":
			black++
		}
	}
	if red != 10 || black != 10 {
		t.Fatalf("initial pieces r=%d b=%d, want 10/10", red, black)
	}
	if len(view.LegalMoves) == 0 {
		t.Fatal("no initial legal moves")
	}
	for _, m := range view.LegalMoves {
		if m.From == nil || m.To == nil || m.From.Index == m.To.Index {
			t.Fatalf("invalid move shape: %+v", m)
		}
	}
}

func TestBoardGeometry(t *testing.T) {
	center := holeByXY[[2]int{8, 0}]
	if holeRow[center] != 8 || holeX2[center] != 0 {
		t.Fatalf("center hole lookup failed: id=%d row=%d x2=%d", center, holeRow[center], holeX2[center])
	}
	if len(neighbors[center]) != 6 {
		t.Fatalf("center hole should have 6 neighbours, got %d", len(neighbors[center]))
	}
	corner := holeByXY[[2]int{0, 0}]
	if len(neighbors[corner]) != 2 {
		t.Fatalf("top corner should have 2 neighbours, got %d", len(neighbors[corner]))
	}
}

func TestJumpMove(t *testing.T) {
	g := &Game{}
	st := g.Initial().(*State)
	moves := genMoves(st)
	jumps := 0
	for _, m := range moves {
		if m.jump {
			jumps++
		}
	}
	if jumps == 0 {
		t.Fatal("initial position should contain at least one jump")
	}
	pos, _, err := g.Apply(st, game.MoveInput{From: intPtr(moves[0].from), To: intPtr(moves[0].to)})
	if err != nil {
		t.Fatal(err)
	}
	if pos.(*State).Turn != Black {
		t.Fatal("turn did not switch")
	}
}

func TestAllHome(t *testing.T) {
	var board [Holes]int8
	for id := range homeBlack {
		board[id] = Red
	}
	if !allHome(board, Red) {
		t.Fatal("all-home win not detected")
	}
	board[holeByXY[[2]int{8, 0}]] = Red
	if allHome(board, Red) {
		t.Fatal("piece outside the goal should prevent the win")
	}
}

func TestSearchReturnsLegal(t *testing.T) {
	g := &Game{}
	res, err := g.Search(context.Background(), g.Initial(), game.SearchOptions{
		Difficulty: "easy", MaxDepth: 1, TimeLimit: time.Second, Seed: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Move == nil || res.Move.From == nil || res.Move.To == nil {
		t.Fatal("search returned no move")
	}
	if _, _, err := g.Apply(g.Initial(), game.MoveInput{From: intPtr(res.Move.From.Index), To: intPtr(res.Move.To.Index)}); err != nil {
		t.Fatalf("search returned illegal move: %v", err)
	}
}

func intPtr(v int) *int { return &v }
