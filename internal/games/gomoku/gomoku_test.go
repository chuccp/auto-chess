package gomoku

import (
	"context"
	"testing"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
)

func TestFiveDirections(t *testing.T) {
	dirs := [][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}}
	for _, d := range dirs {
		var b [Size]int8
		row, col := 7, 7
		for i := 0; i < 5; i++ {
			b[(row+i*d[0])*Cols+col+i*d[1]] = Black
		}
		idx := (row+4*d[0])*Cols + col + 4*d[1]
		if !wouldWin(b, idx, Black) {
			t.Fatalf("direction %v not detected as five", d)
		}
	}
}

func TestApplyWin(t *testing.T) {
	st := &State{Turn: Black, Last: -1}
	for col := 3; col <= 6; col++ {
		st.Board[7*Cols+col] = Black
	}
	st.Moves = 4
	pos, _, err := (&Game{}).Apply(st, game.MoveInput{Index: intPtr(7*Cols + 7)})
	if err != nil {
		t.Fatal(err)
	}
	view := (&Game{}).Encode(pos, false)
	if view.Status.State != "win" || view.Status.Winner != "b" {
		t.Fatalf("five-in-a-row status = %+v", view.Status)
	}
}

func TestSearchWinsImmediately(t *testing.T) {
	st := &State{Turn: Black, Last: -1}
	for col := 3; col <= 6; col++ {
		st.Board[7*Cols+col] = Black
	}
	st.Moves = 4
	res, err := (&Game{}).Search(context.Background(), st, game.SearchOptions{
		Difficulty: "normal", MaxDepth: 2, TimeLimit: time.Second, Seed: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Move == nil || res.Move.Index == nil {
		t.Fatal("no move")
	}
	idx := res.Move.Index.Index
	if idx != 7*Cols+2 && idx != 7*Cols+7 {
		t.Fatalf("AI did not complete five, played %d", idx)
	}
	if !res.GameOver {
		t.Fatal("winning move should end the game")
	}
}

func TestSearchBlocksFour(t *testing.T) {
	st := &State{Turn: Black, Last: -1}
	for col := 3; col <= 6; col++ {
		st.Board[7*Cols+col] = White
	}
	st.Board[7*Cols+2] = Black // close one end
	st.Moves = 5
	res, err := (&Game{}).Search(context.Background(), st, game.SearchOptions{
		Difficulty: "normal", MaxDepth: 2, TimeLimit: time.Second, Seed: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Move == nil || res.Move.Index == nil || res.Move.Index.Index != 7*Cols+7 {
		t.Fatalf("AI failed to block the open end: %+v", res.Move)
	}
}

func TestDecodeValidation(t *testing.T) {
	if _, err := (&Game{}).Decode(game.PositionInput{Board: nil, Turn: "b"}); err == nil {
		t.Fatal("empty board should fail")
	}
}

func intPtr(v int) *int { return &v }

func TestDecodeRejectsInconsistentTurn(t *testing.T) {
	board := make([]string, Size)
	board[0] = "b" // black has one stone, so it is White's turn
	_, err := (&Game{}).Decode(game.PositionInput{Board: board, Turn: "b"})
	if err == nil {
		t.Fatal("turn=b with one black stone should be rejected")
	}
	if _, err := (&Game{}).Decode(game.PositionInput{Board: board, Turn: "w"}); err != nil {
		t.Fatalf("turn=w should be accepted: %v", err)
	}
}
