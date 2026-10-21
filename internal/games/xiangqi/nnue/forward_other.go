//go:build !amd64 || purego

package nnue

const useAVX2 = false

func fc0AVX2(input *uint8, weights *int8, bias *int32, out *int32) {}

func applyDeltaAVX2(values *int32, add, remove *int16, n int) {}

func transformAVX2(values *int32, out *uint8) {}

func addFeatureAVX2(values *int32, add *int16, n int) {}

func applyDelta2AVX2(values *int32, add, remove1, remove2 *int16, n int) {}

func fc1AVX2(input *uint8, weights *int8, bias *int32, out *int32) {}