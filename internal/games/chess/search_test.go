package chess

import (
	"context"
	"testing"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
)

func TestMateInOne(t *testing.T) {
	// White rook a1 to a8 is mate: the black king is boxed in by its own pawns.
	st, err := parseFEN("7k/6pp/8/8/8/8/8/R6K w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	res, err := (&Game{}).Search(context.Background(), st, game.SearchOptions{
		Difficulty: "hard",
		MaxDepth:   3,
		TimeLimit:  2 * time.Second,
		Seed:       1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Move == nil || res.Move.UCI != "a1a8" {
		t.Fatalf("AI missed mate in one: %+v", res.Move)
	}
	if res.Evaluation.MateIn != 1 {
		t.Fatalf("mateIn = %d, want 1", res.Evaluation.MateIn)
	}
	if !res.GameOver {
		t.Fatal("mate should end the game")
	}
}
