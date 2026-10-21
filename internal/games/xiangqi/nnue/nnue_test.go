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
}
