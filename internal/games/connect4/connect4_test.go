package connect4

import (
	"context"
	"testing"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
)

func TestVerticalWin(t *testing.T) {
	g := &Game{}
	pos := g.Initial()
	seq := []int{0, 1, 0, 1, 0, 1, 0}
	for i, col := range seq {
		idx, _ := landing(pos.(*State).Board, col)
		var err error
		pos, _, err = g.Apply(pos, game.MoveInput{Index: &idx})
		if err != nil {
			t.Fatalf("move %d: %v", i, err)
		}
	}
	view := g.Encode(pos, false)
	if view.Status.State != "win" || view.Status.Winner != "r" {
		t.Fatalf("vertical four status = %+v", view.Status)
	}
}

func TestSearchTakesWin(t *testing.T) {
	st := &State{Turn: Red, Last: -1}
	for row := 2; row <= 4; row++ {
		st.Board[row*Cols+0] = Red
	}
	st.Moves = 3
	// Yellow has three in another column; red must win first.
	st.Board[5*Cols+6] = Yellow
	st.Board[4*Cols+6] = Yellow
	st.Board[3*Cols+6] = Yellow
	st.Moves = 6
	res, err := (&Game{}).Search(context.Background(), st, game.SearchOptions{
		Difficulty: "normal", MaxDepth: 4, TimeLimit: time.Second, Seed: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Move == nil || res.Move.Index == nil || res.Move.Index.Col != 0 {
		t.Fatalf("AI missed the immediate vertical win: %+v", res.Move)
	}
	if !res.GameOver {
		t.Fatal("winning move should end the game")
	}
}

func TestInvalidDrop(t *testing.T) {
	g := &Game{}
	if _, _, err := g.Apply(g.Initial(), game.MoveInput{Index: intPtr(0)}); err == nil {
		t.Fatal("top-row landing should be illegal on an empty board")
	}
}

func intPtr(v int) *int { return &v }
