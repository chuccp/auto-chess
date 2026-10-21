package xiangqi

import (
	"testing"
)

func TestPerftInitial(t *testing.T) {
	st, err := parseFEN("rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	if got := perft(st, 1); got != 44 {
		t.Fatalf("perft(1) = %d, want 44", got)
	}
	if got := perft(st, 2); got != 1920 {
		t.Fatalf("perft(2) = %d, want 1920", got)
	}
}

func perft(st *State, depth int) int {
	if depth == 0 {
		return 1
	}
	moves := legalMoves(st)
	if depth == 1 {
		return len(moves)
	}
	total := 0
	for _, m := range moves {
		total += perft(makeMove(st, m), depth-1)
	}
	return total
}

func TestCannonCapture(t *testing.T) {
	st, err := parseFEN("rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range legalMoves(st) {
		if m.From == 19 && m.To == 82 {
			found = true
		}
	}
	if !found {
		t.Fatal("cannon b2xb9 over the b7 screen should be legal")
	}
}

func TestHorseLeg(t *testing.T) {
	st, err := parseFEN("rnbakabnr/9/1c5c1/p1p1p1p1p/9/9/P1P1P1P1P/1C5C1/9/RNBAKABNR w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	// Block the b-file leg of the b0 horse.
	st.Board[10] = Pawn
	moves := legalMoves(st)
	for _, m := range moves {
		if m.From == 1 && (m.To == 18 || m.To == 20) {
			t.Fatalf("horse moved through a blocked leg: %v", m)
		}
	}
}

func TestFlyingGeneral(t *testing.T) {
	// A red advisor on e1 blocks the two kings. Any advisor move exposes the
	// file and must be rejected.
	st, err := parseFEN("4k4/9/9/9/9/9/9/9/4A4/4K4 w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range legalMoves(st) {
		if m.From == 40 {
			t.Fatalf("advisor moved and exposed the kings: %v", m)
		}
	}
}

func TestFENRoundTrip(t *testing.T) {
	in := "r1bakab1r/9/1cn3nc1/p1p1p1p1p/9/9/P1P1P1P1P/1CN3NC1/9/R1BAKAB1R w - - 0 1"
	st, err := parseFEN(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.fen(); got != in {
		t.Fatalf("FEN round trip:\n got %s\nwant %s", got, in)
	}
}

func TestIncrementalHash(t *testing.T) {
	st := New().Initial().(*State)
	for ply := 0; ply < 4; ply++ {
		moves := legalMoves(st)
		if len(moves) == 0 {
			break
		}
		st = makeMove(st, moves[0])
		want := calcHash(st.Board, st.Turn)
		if st.hash != want {
			t.Fatalf("ply %d: hash %#x, want %#x", ply, st.hash, want)
		}
	}
}
