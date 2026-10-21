package nnue

import (
	"os"
	"testing"
)

func TestLoadAndEvaluate(t *testing.T) {
	path := os.Getenv("PIKAFISH_NNUE")
	if path == "" {
		t.Skip("set PIKAFISH_NNUE to a Pikafish 2024 .nnue file")
	}
	n, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	board := []string{
		"rR", "rN", "rB", "rA", "rK", "rA", "rB", "rN", "rR",
		"", "", "", "", "", "", "", "", "",
		"", "rC", "", "", "", "", "", "rC", "",
		"rP", "", "rP", "", "rP", "", "rP", "", "rP",
		"", "", "", "", "", "", "", "", "",
		"", "", "", "", "", "", "", "", "",
		"bP", "", "bP", "", "bP", "", "bP", "", "bP",
		"", "bC", "", "", "", "", "", "bC", "",
		"", "", "", "", "", "", "", "", "",
		"bR", "bN", "bB", "bA", "bK", "bA", "bB", "bN", "bR",
	}
	got, err := n.Evaluate(board, "w")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("description=%q size=%d eval=%d", n.Description(), n.FileSize(), got)
	simple := make([]string, 90)
	simple[4] = "rK"
	simple[85] = "bK"
	simple[4*9+4] = "rP"
	s, err := n.Evaluate(simple, "w")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("simple eval=%d", s)

	inc := append([]string(nil), board...)
	inc[27], inc[36] = "", "rP"
	oldPieces := make([]int, 90)
	newPieces := make([]int, 90)
	for i, cell := range board {
		oldPieces[i], _ = pieceCode(cell)
	}
	for i, cell := range inc {
		newPieces[i], _ = pieceCode(cell)
	}
	fullAcc, err := n.NewAccumulator(newPieces)
	if err != nil {
		t.Fatal(err)
	}
	startAcc, err := n.NewAccumulator(oldPieces)
	if err != nil {
		t.Fatal(err)
	}
	updated := n.UpdateAccumulator(startAcc, oldPieces, newPieces)
	fullEval, err := n.EvaluateAccumulator(fullAcc, "w")
	if err != nil {
		t.Fatal(err)
	}
	updatedEval, err := n.EvaluateAccumulator(updated, "w")
	if err != nil {
		t.Fatal(err)
	}
	if fullEval != updatedEval {
		t.Fatalf("incremental eval = %d, full = %d", updatedEval, fullEval)
	}
	t.Logf("incremental eval=%d", updatedEval)
}

func int8Board(board []string) [90]int8 {
	var out [90]int8
	for i, cell := range board {
		if cell == "" {
			continue
		}
		var p int8
		switch cell[1] {
		case 'R':
			p = 5
		case 'N':
			p = 4
		case 'B':
			p = 3
		case 'A':
			p = 2
		case 'K':
			p = 1
		case 'C':
			p = 6
		case 'P':
			p = 7
		}
		if cell[0] == 'b' {
			p = -p
		}
		out[i] = p
	}
	return out
}

func TestUpdateMoveMatchesFull(t *testing.T) {
	path := os.Getenv("PIKAFISH_NNUE")
	if path == "" {
		t.Skip("set PIKAFISH_NNUE")
	}
	n, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	board := []string{
		"rR", "rN", "rB", "rA", "rK", "rA", "rB", "rN", "rR",
		"", "", "", "", "", "", "", "", "",
		"", "rC", "", "", "", "", "", "rC", "",
		"rP", "", "rP", "", "rP", "", "rP", "", "rP",
		"", "", "", "", "", "", "", "", "",
		"", "", "", "", "", "", "", "", "",
		"bP", "", "bP", "", "bP", "", "bP", "", "bP",
		"", "bC", "", "", "", "", "", "bC", "",
		"", "", "", "", "", "", "", "", "",
		"bR", "bN", "bB", "bA", "bK", "bA", "bB", "bN", "bR",
	}
	old := int8Board(board)
	start, err := n.NewAccumulator(func() []int {
		var p [90]int
		for i, v := range old {
			p[i] = codeFromInt8(v)
		}
		return p[:]
	}())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct{ from, to int }{
		{19, 82}, // cannon captures knight
		{4, 13},  // king step
		{27, 36}, // pawn step
	}
	for _, tc := range tests {
		next := old
		next[tc.to] = next[tc.from]
		next[tc.from] = 0
		var pieces [90]int
		for i, v := range next {
			pieces[i] = codeFromInt8(v)
		}
		full, err := n.NewAccumulator(pieces[:])
		if err != nil {
			t.Fatal(err)
		}
		updated := n.UpdateMove(start, old, tc.from, tc.to)
		want, err := n.EvaluateAccumulator(full, "w")
		if err != nil {
			t.Fatal(err)
		}
		got, err := n.EvaluateAccumulator(updated, "w")
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("move %d-%d: got %d want %d", tc.from, tc.to, got, want)
		}
	}
}
