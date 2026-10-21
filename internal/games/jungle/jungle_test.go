package jungle

import (
	"testing"

	"github.com/chuccp/auto-chess/internal/game"
)

func TestInitialPieces(t *testing.T) {
	g := &Game{}
	view := g.Encode(g.Initial(), true)
	count := 0
	for _, cell := range view.Board {
		if cell != "" {
			count++
		}
	}
	if count != 16 {
		t.Fatalf("initial piece count = %d, want 16", count)
	}
	if view.Board[8*Cols+0] != "rT" {
		t.Fatalf("red tiger not at bottom-left: %q", view.Board[8*Cols+0])
	}
	if view.Board[0] != "bL" {
		t.Fatalf("black lion not at top-left: %q", view.Board[0])
	}
	if len(view.LegalMoves) == 0 {
		t.Fatal("no initial legal moves")
	}
}

func TestRatElephantRules(t *testing.T) {
	// Rat captures elephant from land.
	board := [Size]int8{}
	board[6*Cols+4] = piece(Red, Rat)
	board[6*Cols+5] = piece(Black, Elephant)
	if !canCapture(board, 6*Cols+4, piece(Red, Rat), 6*Cols+5) {
		t.Fatal("rat should capture elephant on land")
	}
	// Elephant cannot capture rat.
	if canCapture(board, 6*Cols+5, piece(Black, Elephant), 6*Cols+4) {
		t.Fatal("elephant should not capture a rat")
	}
	// Rat is safe in water from a land elephant.
	waterBoard := [Size]int8{}
	waterBoard[3*Cols+1] = piece(Red, Rat) // water
	waterBoard[3*Cols+0] = piece(Black, Elephant)
	if canCapture(waterBoard, 3*Cols+0, piece(Black, Elephant), 3*Cols+1) {
		t.Fatal("elephant should not capture a rat in water")
	}
}

func TestLionJump(t *testing.T) {
	st := &State{Turn: Red, LastFrom: -1, LastTo: -1}
	st.Board[2*Cols+1] = piece(Red, Lion)
	found := false
	for _, m := range genMoves(st) {
		if m.from == 2*Cols+1 && m.to == 6*Cols+1 {
			found = true
		}
	}
	if !found {
		t.Fatal("lion should be able to jump over the water")
	}
	// A rat in the water blocks the jump.
	st.Board[4*Cols+1] = piece(Black, Rat)
	for _, m := range genMoves(st) {
		if m.from == 2*Cols+1 && m.to == 6*Cols+1 {
			t.Fatal("lion jumped over a rat in the water")
		}
	}
}

func TestEnterDenWins(t *testing.T) {
	g := &Game{}
	st := &State{Turn: Red, LastFrom: -1, LastTo: -1}
	st.Board[1*Cols+3] = piece(Red, Lion)
	pos, _, err := g.Apply(st, game.MoveInput{From: intPtr(1*Cols + 3), To: intPtr(3)})
	if err != nil {
		t.Fatal(err)
	}
	view := g.Encode(pos, false)
	if view.Status.State != "win" || view.Status.Winner != "r" {
		t.Fatalf("entering the den did not win: %+v", view.Status)
	}
}

func TestTrapWeakens(t *testing.T) {
	board := [Size]int8{}
	board[7*Cols+2] = piece(Red, Cat)
	board[7*Cols+3] = piece(Black, Elephant) // red trap
	if !canCapture(board, 7*Cols+2, piece(Red, Cat), 7*Cols+3) {
		t.Fatal("cat should capture an elephant stuck in the red trap")
	}
}

func intPtr(v int) *int { return &v }
