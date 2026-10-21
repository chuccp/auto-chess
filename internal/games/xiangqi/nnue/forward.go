package nnue

const (
	fc1Inputs = 32
	fcSkip    = 15
)

func (l layer) forwardInto(input []uint8, out []int32) {
	copy(out[:l.out], l.bias)
	for i := 0; i < l.in && i < len(input); i++ {
		v := input[i]
		if v == 0 {
			continue
		}
		iv := int32(v)
		for o := 0; o < l.out; o++ {
			out[o] += int32(l.weights[o*l.in+i]) * iv
		}
	}
}

func transposeFC0(l layer) []int8 {
	out := make([]int8, l.in*l.out)
	for o := 0; o < l.out; o++ {
		row := l.weights[o*l.in : (o+1)*l.in]
		for i, w := range row {
			out[i*l.out+o] = w
		}
	}
	return out
}

func (s stack) forwardFC0(input [ftHalf]uint8, out []int32) {
	if useAVX2 && len(s.fc0.weights) == ftHalf*(fc0Outputs+1) && len(s.fc0.bias) >= fc0Outputs+1 && len(out) >= fc0Outputs+1 {
		fc0AVX2(&input[0], &s.fc0.weights[0], &s.fc0.bias[0], &out[0])
		return
	}
	copy(out[:s.fc0.out], s.fc0.bias)
	for i, v := range input {
		if v == 0 {
			continue
		}
		iv := int32(v)
		row := s.fc0T[i*s.fc0.out : (i+1)*s.fc0.out]
		for o, w := range row {
			out[o] += int32(w) * iv
		}
	}
}

func (s stack) propagate(transformed [ftHalf]uint8) int32 {
	var fc0 [fc0Outputs + 1]int32
	s.forwardFC0(transformed, fc0[:])

	// 2024 network: sqr-clipped first 15 outputs followed by clipped ReLU of
	// the same 15 outputs. fc0[15] is the skip connection.
	var concat [fc1Inputs]uint8
	for i := 0; i < fc0Outputs; i++ {
		v := int64(fc0[i])
		sq := v * v >> (2*weightScale + 7)
		if sq > 127 {
			sq = 127
		}
		concat[i] = uint8(sq)
		concat[fc0Outputs+i] = uint8(clamp(int(fc0[i])>>weightScale, 0, 127))
	}
	var fc1 [fc1Outputs]int32
	s.fc1.forwardInto(concat[:], fc1[:])
	var ac1 [fc1Outputs]uint8
	for i, v := range fc1 {
		ac1[i] = uint8(clamp(int(v)>>weightScale, 0, 127))
	}
	var fc2 [1]int32
	s.fc2.forwardInto(ac1[:], fc2[:])
	skip := int64(fc0[fcSkip]) * (600 * outputScale) / outputDivisor
	return fc2[0] + int32(skip)
}
