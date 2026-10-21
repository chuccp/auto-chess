package games_test

import (
	"context"
	"testing"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
	"github.com/chuccp/auto-chess/internal/registry"
)

func moveInput(m game.Move) game.MoveInput {
	if m.UCI != "" {
		return game.MoveInput{UCI: m.UCI}
	}
	if m.Index != nil {
		i := m.Index.Index
		return game.MoveInput{Index: &i}
	}
	if m.From != nil && m.To != nil {
		from, to := m.From.Index, m.To.Index
		return game.MoveInput{From: &from, To: &to}
	}
	return game.MoveInput{}
}

func TestInitialPositionsAndRoundTrip(t *testing.T) {
	reg := registry.New()
	if len(reg.IDs()) != 7 {
		t.Fatalf("expected 7 games, got %d", len(reg.IDs()))
	}
	for _, id := range reg.IDs() {
		id := id
		t.Run(id, func(t *testing.T) {
			g, ok := reg.Get(id)
			if !ok {
				t.Fatalf("game %s not found", id)
			}
			meta := g.Meta()
			pos := g.Initial()
			view := g.Encode(pos, true)
			if len(view.Board) != meta.Board.Cells {
				t.Fatalf("board has %d cells, metadata says %d", len(view.Board), meta.Board.Cells)
			}
			if view.LegalMoveCount == 0 || len(view.LegalMoves) == 0 {
				t.Fatalf("initial position has no legal moves")
			}
			if view.Turn == "" || view.Status.State == "" {
				t.Fatalf("missing turn or status: %+v", view)
			}
			in := game.PositionInput{FEN: view.FEN, Board: view.Board, Turn: view.Turn}
			pos2, err := g.Decode(in)
			if err != nil {
				t.Fatalf("decode initial view: %v", err)
			}
			view2 := g.Encode(pos2, false)
			if len(view2.Board) != len(view.Board) {
				t.Fatalf("round trip board length changed")
			}
			for i := range view.Board {
				if view.Board[i] != view2.Board[i] {
					t.Fatalf("round trip board differs at %d: %q != %q", i, view.Board[i], view2.Board[i])
				}
			}
		})
	}
}

func TestSearchAndApplyAllGames(t *testing.T) {
	reg := registry.New()
	for _, id := range reg.IDs() {
		id := id
		t.Run(id, func(t *testing.T) {
			g, _ := reg.Get(id)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var progress []game.SearchProgress
			res, err := g.Search(ctx, g.Initial(), game.SearchOptions{
				Difficulty: "easy",
				MaxDepth:   1,
				TimeLimit:  500 * time.Millisecond,
				Seed:       1,
				Progress: func(p game.SearchProgress) {
					progress = append(progress, p)
				},
			})
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if len(progress) == 0 {
				t.Fatalf("search did not emit progress")
			}
			if progress[0].Depth < 1 {
				t.Fatalf("progress depth = %d", progress[0].Depth)
			}
			if res.Move == nil {
				t.Fatalf("search returned no move: %+v", res)
			}
			view := g.Encode(g.Initial(), true)
			legal := false
			for _, m := range view.LegalMoves {
				if m.UCI != "" && m.UCI == res.Move.UCI {
					legal = true
				}
				if m.Index != nil && res.Move.Index != nil && m.Index.Index == res.Move.Index.Index {
					legal = true
				}
				if m.From != nil && res.Move.From != nil && m.From.Index == res.Move.From.Index && m.To.Index == res.Move.To.Index {
					legal = true
				}
			}
			if !legal {
				t.Fatalf("search returned illegal move: %+v", res.Move)
			}
			if _, _, err := g.Apply(g.Initial(), moveInput(*res.Move)); err != nil {
				t.Fatalf("apply returned move: %v", err)
			}
		})
	}
}
