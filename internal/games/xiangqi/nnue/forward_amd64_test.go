//go:build amd64 && !purego

package nnue

import (
	"math/rand"
	"testing"
)

func TestFC0AVX2MatchesScalar(t *testing.T) {
	if !useAVX2 {
		t.Skip("AVX2 unavailable")
	}
	rng := rand.New(rand.NewSource(1))
	var input [ftHalf]uint8
	for i := range input {
		input[i] = uint8(rng.Intn(128))
	}
	weights := make([]int8, (fc0Outputs+1)*ftHalf)
	for i := range weights {
		weights[i] = int8(rng.Intn(256) - 128)
	}
	bias := make([]int32, fc0Outputs+1)
	for i := range bias {
		bias[i] = int32(rng.Intn(2001) - 1000)
	}
	want := make([]int32, fc0Outputs+1)
	copy(want, bias)
	for i, v := range input {
		if v == 0 {
			continue
		}
		for o := 0; o < fc0Outputs+1; o++ {
			want[o] += int32(weights[o*ftHalf+i]) * int32(v)
		}
	}
	got := make([]int32, fc0Outputs+1)
	fc0AVX2(&input[0], &weights[0], &bias[0], &got[0])
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("output %d: got %d want %d", i, got[i], want[i])
		}
	}
}
