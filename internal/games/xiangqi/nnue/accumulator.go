package nnue

import "errors"

// Accumulator is the feature-transformer state for both perspectives.
type Accumulator struct {
	Values [2][ftHalf]int32
	PSQT   [2][psqtBuckets]int32
	Kings  [2]int
	AB     [2]int
	Count  int
}

// NewAccumulator builds the feature transformer state from a 90-square board.
func (n *Network) NewAccumulator(pieces []int) (*Accumulator, error) {
	if len(pieces) != 90 {
		return nil, errors.New("nnue: board must have 90 squares")
	}
	acc := &Accumulator{Count: 0}
	for _, p := range pieces {
		if p != 0 {
			acc.Count++
		}
	}
	for perspective := 0; perspective < 2; perspective++ {
		n.rebuildPerspective(acc, pieces, perspective)
	}
	return acc, nil
}

func (n *Network) rebuildPerspective(acc *Accumulator, pieces []int, perspective int) {
	for i := 0; i < ftHalf; i++ {
		acc.Values[perspective][i] = int32(n.bias[i])
	}
	for i := 0; i < psqtBuckets; i++ {
		acc.PSQT[perspective][i] = 0
	}
	ksq, ab := kingAndAB(pieces, perspective)
	acc.Kings[perspective] = ksq
	acc.AB[perspective] = ab
	if ksq < 0 {
		return
	}
	for sq, pc := range pieces {
		if pc != 0 {
			n.addFeature(&acc.Values[perspective], &acc.PSQT[perspective], featureIndex(sq, pc, ksq, ab, perspective))
		}
	}
}

func (n *Network) addFeature(values *[ftHalf]int32, psqt *[psqtBuckets]int32, index int) {
	base := index * ftHalf
	for j := 0; j < ftHalf; j++ {
		values[j] += int32(n.weights[base+j])
	}
	pbase := index * psqtBuckets
	for j := 0; j < psqtBuckets; j++ {
		psqt[j] += n.psqt[pbase+j]
	}
}

func (n *Network) removeFeature(values *[ftHalf]int32, psqt *[psqtBuckets]int32, index int) {
	base := index * ftHalf
	for j := 0; j < ftHalf; j++ {
		values[j] -= int32(n.weights[base+j])
	}
	pbase := index * psqtBuckets
	for j := 0; j < psqtBuckets; j++ {
		psqt[j] -= n.psqt[pbase+j]
	}
}

// UpdateAccumulator copies acc and applies the changed pieces. A full refresh
// is done only when the own king or the advisor/bishop count changes.
func (n *Network) UpdateAccumulator(acc *Accumulator, oldPieces, newPieces []int) *Accumulator {
	next := *acc
	next.Count = 0
	for _, p := range newPieces {
		if p != 0 {
			next.Count++
		}
	}
	for perspective := 0; perspective < 2; perspective++ {
		newKing, newAB := kingAndAB(newPieces, perspective)
		if newKing != acc.Kings[perspective] || newAB != acc.AB[perspective] {
			n.rebuildPerspective(&next, newPieces, perspective)
			continue
		}
		for sq := 0; sq < 90; sq++ {
			if oldPieces[sq] == newPieces[sq] {
				continue
			}
			if oldPieces[sq] != 0 {
				n.removeFeature(&next.Values[perspective], &next.PSQT[perspective],
					featureIndex(sq, oldPieces[sq], acc.Kings[perspective], acc.AB[perspective], perspective))
			}
			if newPieces[sq] != 0 {
				n.addFeature(&next.Values[perspective], &next.PSQT[perspective],
					featureIndex(sq, newPieces[sq], newKing, newAB, perspective))
			}
		}
	}
	return &next
}

// EvaluateAccumulator evaluates a precomputed accumulator from the given turn.
func (n *Network) EvaluateAccumulator(acc *Accumulator, turn string) (int, error) {
	if acc == nil {
		return 0, errors.New("nnue: nil accumulator")
	}
	if acc.Count == 0 {
		return 0, errors.New("nnue: empty board")
	}
	if turn != "w" && turn != "b" {
		return 0, errors.New("nnue: turn must be w or b")
	}
	side := 0
	if turn == "b" {
		side = 1
	}
	bucket := (acc.Count - 1) / 4
	if bucket >= layerStacks {
		bucket = layerStacks - 1
	}
	var transformed [ftHalf]uint8
	for p := 0; p < 2; p++ {
		offset := (ftHalf / 2) * p
		for j := 0; j < ftHalf/2; j++ {
			a := clamp(int(acc.Values[p][j]), 0, 127)
			b := clamp(int(acc.Values[p][j+ftHalf/2]), 0, 127)
			transformed[offset+j] = uint8(a * b / 128)
		}
	}
	psqtEval := int32((acc.PSQT[side][bucket] - acc.PSQT[1-side][bucket]) / 2)
	positional := n.layers[bucket].propagate(transformed)
	return int((psqtEval + positional) / outputScale), nil
}
