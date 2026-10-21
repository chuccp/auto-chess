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

func TestApplyDeltaAVX2(t *testing.T) {
	if !useAVX2 {
		t.Skip("AVX2 unavailable")
	}
	rng := rand.New(rand.NewSource(2))
	values := make([]int32, ftHalf)
	want := make([]int32, ftHalf)
	for i := range values {
		values[i] = int32(rng.Intn(2001) - 1000)
		want[i] = values[i]
	}
	add := make([]int16, ftHalf)
	remove := make([]int16, ftHalf)
	for i := range add {
		add[i] = int16(rng.Intn(2001) - 1000)
		remove[i] = int16(rng.Intn(2001) - 1000)
		want[i] += int32(add[i]) - int32(remove[i])
	}
	applyDeltaAVX2(&values[0], &add[0], &remove[0], ftHalf)
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("index %d: got %d want %d", i, values[i], want[i])
		}
	}
}

func TestTransformAVX2(t *testing.T) {
	if !useAVX2 {
		t.Skip("AVX2 unavailable")
	}
	rng := rand.New(rand.NewSource(3))
	var values [2][ftHalf]int32
	for p := 0; p < 2; p++ {
		for i := 0; i < ftHalf; i++ {
			values[p][i] = int32(rng.Intn(2401) - 1200)
		}
	}
	var got [ftHalf]uint8
	transformAVX2(&values[0][0], &got[0])
	for p := 0; p < 2; p++ {
		for j := 0; j < ftHalf/2; j++ {
			clamp := func(v int32) int {
				if v < 0 {
					return 0
				}
				if v > 127 {
					return 127
				}
				return int(v)
			}
			want := uint8(clamp(values[p][j]) * clamp(values[p][j+ftHalf/2]) / 128)
			if got[p*(ftHalf/2)+j] != want {
				t.Fatalf("p=%d j=%d got=%d want=%d", p, j, got[p*(ftHalf/2)+j], want)
			}
		}
	}
}

func TestFC1AVX2MatchesScalar(t *testing.T) {
	if !useAVX2 {
		t.Skip("AVX2 unavailable")
	}
	rng := rand.New(rand.NewSource(4))
	var input [fc1Inputs]uint8
	for i := range input {
		input[i] = uint8(rng.Intn(128))
	}
	weights := make([]int8, fc1Inputs*fc1Outputs)
	for i := range weights {
		weights[i] = int8(rng.Intn(256) - 128)
	}
	bias := make([]int32, fc1Outputs)
	for i := range bias {
		bias[i] = int32(rng.Intn(2001) - 1000)
	}
	want := make([]int32, fc1Outputs)
	copy(want, bias)
	for o := 0; o < fc1Outputs; o++ {
		for i, v := range input {
			want[o] += int32(weights[o*fc1Inputs+i]) * int32(v)
		}
	}
	got := make([]int32, fc1Outputs)
	fc1AVX2(&input[0], &weights[0], &bias[0], &got[0])
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("output %d: got %d want %d", i, got[i], want[i])
		}
	}
}
