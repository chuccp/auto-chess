package nnue

import (
	"os"
	"testing"
)

func BenchmarkEvaluateAccumulator(b *testing.B) {
	path := os.Getenv("PIKAFISH_NNUE")
	if path == "" {
		b.Skip("set PIKAFISH_NNUE")
	}
	n, err := Load(path)
	if err != nil {
		b.Fatal(err)
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
	pieces := make([]int, 90)
	for i, cell := range board {
		pieces[i], _ = pieceCode(cell)
	}
	acc, err := n.NewAccumulator(pieces)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := n.EvaluateAccumulator(acc, "w"); err != nil {
			b.Fatal(err)
		}
	}
}
