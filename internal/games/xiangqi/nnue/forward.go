package nnue

const (
	fc1Inputs = 32
	fcSkip    = 15
)

func (l layer) forward(input []uint8) []int32 {
	out := make([]int32, l.out)
	copy(out, l.bias)
	for o := 0; o < l.out; o++ {
		row := l.weights[o*l.in : (o+1)*l.in]
		for i := 0; i < l.in && i < len(input); i++ {
			if input[i] == 0 {
				continue
			}
			out[o] += int32(row[i]) * int32(input[i])
		}
	}
	return out
}

func (s stack) propagate(transformed [ftHalf]uint8) int32 {
	fc0 := s.fc0.forward(transformed[:])

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
	fc1 := s.fc1.forward(concat[:])
	var ac1 [fc1Outputs]uint8
	for i, v := range fc1 {
		ac1[i] = uint8(clamp(int(v)>>weightScale, 0, 127))
	}
	fc2 := s.fc2.forward(ac1[:])
	skip := int64(fc0[fcSkip]) * (600 * outputScale) / outputDivisor
	return fc2[0] + int32(skip)
}
