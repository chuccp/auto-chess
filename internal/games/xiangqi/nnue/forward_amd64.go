//go:build amd64 && !purego

package nnue

import "golang.org/x/sys/cpu"

var useAVX2 = cpu.X86.HasAVX2

//go:noescape
func fc0AVX2(input *uint8, weights *int8, bias *int32, out *int32)

//go:noescape
func applyDeltaAVX2(values *int32, add, remove *int16, n int)

//go:noescape
func transformAVX2(values *int32, out *uint8)
