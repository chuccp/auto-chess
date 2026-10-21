package chess

import (
	"testing"
)

func TestPerftInitial(t *testing.T) {
	st, err := parseFEN("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		depth int
		want  int
	}{
		{1, 20},
		{2, 400},
		{3, 8902},
	}
	for _, c := range cases {
		if got := perft(st, c.depth); got != c.want {
			t.Fatalf("perft(%d) = %d, want %d", c.depth, got, c.want)
		}
	}
}

func TestPerftKiwipete(t *testing.T) {
	st, err := parseFEN("r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	if got := perft(st, 2); got != 2039 {
		t.Fatalf("kiwipete perft(2) = %d, want 2039", got)
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

func TestCastling(t *testing.T) {
	st, err := parseFEN("r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	moves := legalMoves(st)
	found := 0
	for _, m := range moves {
		if m.From == 4 && (m.To == 6 || m.To == 2) {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("expected 2 castling moves, found %d", found)
	}
	next := makeMove(st, move{From: 4, To: 6})
	if next.Board[6] != King || next.Board[5] != Rook || next.Board[4] != 0 || next.Board[7] != 0 {
		t.Fatalf("white kingside castling failed: %v", next.Board)
	}
	if next.Castling&(castleWK|castleWQ) != 0 {
		t.Fatalf("white castling rights not cleared: %b", next.Castling)
	}
}

func TestEnPassant(t *testing.T) {
	st, err := parseFEN("rnbqkbnr/ppp1pppp/8/3pP3/8/8/PPPP1PPP/RNBQKBNR w KQkq d6 0 3")
	if err != nil {
		t.Fatal(err)
	}
	next := makeMove(st, move{From: 36, To: 43})
	if next.Board[43] != Pawn {
		t.Fatalf("en passant pawn did not land on d6")
	}
	if next.Board[35] != 0 {
		t.Fatalf("captured pawn on d5 was not removed")
	}
}

func TestPromotion(t *testing.T) {
	st, err := parseFEN("8/P7/8/8/8/8/8/k6K w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	next := makeMove(st, move{From: 48, To: 56, Promotion: Queen})
	if next.Board[56] != Queen {
		t.Fatalf("promotion failed: %v", next.Board[56])
	}
}

func TestCheckmateAndStalemate(t *testing.T) {
	mate, err := parseFEN("rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 1 3")
	if err != nil {
		t.Fatal(err)
	}
	if st := (&Game{}).status(mate); st.State != "checkmate" || st.Winner != "b" {
		t.Fatalf("fool's mate status = %+v", st)
	}
	stale, err := parseFEN("7k/5Q2/6K1/8/8/8/8/8 b - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	if st := (&Game{}).status(stale); st.State != "stalemate" {
		t.Fatalf("stalemate status = %+v", st)
	}
}

func TestFENRoundTrip(t *testing.T) {
	in := "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1"
	st, err := parseFEN(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.fen(); got != in {
		t.Fatalf("FEN round trip:\n got %s\nwant %s", got, in)
	}
}
