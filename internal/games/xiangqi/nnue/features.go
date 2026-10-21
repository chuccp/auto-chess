package nnue

// HalfKAv2_hm feature encoding, ported from Pikafish 2024.
const (
	psNone    = 0
	psWRook   = 0 * 90
	psBRook   = 1 * 90
	psWCannon = 2 * 90
	psBCannon = 3 * 90
	psWKnight = 4 * 90
	psBKnight = 5 * 90
	psABW     = 6 * 90
	psBKP     = 7 * 90
	psNB      = 8 * 90
)

var pieceSquare = [2][16]int{
	{
		0, psWRook, psABW, psWCannon, psABW, psWKnight, psABW, psABW,
		0, psBRook, psABW, psBCannon, psBKP, psBKnight, psABW, psBKP,
	},
	{
		0, psBRook, psABW, psBCannon, psBKP, psBKnight, psABW, psBKP,
		0, psWRook, psABW, psWCannon, psABW, psWKnight, psABW, psABW,
	},
}

var kingBuckets = [90]uint8{
	0, 0, 0, 0, 1, 8, 0, 0, 0,
	0, 0, 0, 3, 2, 11, 0, 0, 0,
	0, 0, 0, 4, 5, 12, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 4, 5, 12, 0, 0, 0,
	0, 0, 0, 3, 2, 11, 0, 0, 0,
	0, 0, 0, 0, 1, 8, 0, 0, 0,
}

var abMap = [90]uint8{
	0, 0, 0, 1, 0, 2, 5, 0, 0,
	0, 0, 0, 0, 6, 0, 0, 0, 0,
	7, 0, 0, 8, 9, 10, 0, 0, 11,
	0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 14, 0, 0, 0, 15, 0, 0,
	0, 0, 16, 0, 0, 0, 17, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0,
	18, 0, 0, 19, 20, 23, 0, 0, 24,
	0, 0, 0, 0, 25, 0, 0, 0, 0,
	0, 0, 26, 28, 0, 30, 32, 0, 0,
}

var indexMap [2][2][2][90]uint8

func init() {
	for m := 0; m < 2; m++ {
		for r := 0; r < 2; r++ {
			for ab := 0; ab < 2; ab++ {
				for s := 0; s < 90; s++ {
					ss := s
					if m == 1 {
						ss = (ss/9)*9 + (8 - ss%9)
					}
					if r == 1 {
						ss = (9-ss/9)*9 + ss%9
					}
					if ab == 1 {
						ss = int(abMap[ss])
					}
					indexMap[m][r][ab][s] = uint8(ss)
				}
			}
		}
	}
}

func featureIndex(square, piece, kingSquare, ab, perspective int) int {
	kb := kingBuckets[kingSquare]
	mirror := int(kb >> 3)
	rotate := 0
	if perspective == 1 {
		rotate = 1
	}
	abFeature := 0
	switch pieceType(piece) {
	case "A", "B":
		abFeature = 1
	}
	mapped := int(indexMap[mirror][rotate][abFeature][square])
	return mapped + pieceSquare[perspective][piece] + psNB*((int(kb&7)*9)+ab)
}
