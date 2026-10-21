package xiangqi

var (
	zobristPieces [16][Size]uint64
	zobristTurn   uint64
)

func init() {
	seed := uint64(0x9E3779B97F4A7C15)
	next := func() uint64 {
		seed += 0x9E3779B97F4A7C15
		z := seed
		z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
		z = (z ^ (z >> 27)) * 0x94D049BB133111EB
		return z ^ (z >> 31)
	}
	for p := 0; p < 16; p++ {
		for s := 0; s < Size; s++ {
			zobristPieces[p][s] = next()
		}
	}
	zobristTurn = next()
}

func calcHash(board [Size]int8, turn int8) uint64 {
	var h uint64
	for s, p := range board {
		if p != 0 {
			h ^= zobristPieces[int(p)+8][s]
		}
	}
	if turn == Black {
		h ^= zobristTurn
	}
	return h
}
