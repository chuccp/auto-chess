//go:build !amd64 || purego

package nnue

const useAVX2 = false

func fc0AVX2(input *uint8, weights *int8, bias *int32, out *int32) {}
