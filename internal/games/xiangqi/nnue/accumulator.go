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
		var add, remove [4]int
		nAdd, nRemove := 0, 0
		for sq := 0; sq < 90; sq++ {
			if oldPieces[sq] == newPieces[sq] {
				continue
			}
			if oldPieces[sq] != 0 {
				remove[nRemove] = featureIndex(sq, oldPieces[sq], acc.Kings[perspective], acc.AB[perspective], perspective)
				nRemove++
			}
			if newPieces[sq] != 0 {
				add[nAdd] = featureIndex(sq, newPieces[sq], newKing, newAB, perspective)
				nAdd++
			}
		}
		if nAdd != 0 || nRemove != 0 {
			n.applyDelta(&next.Values[perspective], &next.PSQT[perspective], add[:nAdd], remove[:nRemove])
		}
	}
	return &next
}

func codeFromInt8(p int8) int {
	if p == 0 {
		return 0
	}
	base := 0
	if p < 0 {
		base = 8
		p = -p
	}
	var code int
	switch p {
	case 5:
		code = 1 // rook
	case 2:
		code = 2 // advisor
	case 6:
		code = 3 // cannon
	case 7:
		code = 4 // pawn
	case 4:
		code = 5 // knight
	case 3:
		code = 6 // bishop
	case 1:
		code = 7 // king
	}
	return base + code
}

// UpdateMove updates an accumulator for a single move without scanning the
// whole board. The caller passes the board before the move.
func (n *Network) UpdateMove(acc *Accumulator, board [90]int8, from, to int) *Accumulator {
	next := *acc
	moved := board[from]
	captured := board[to]
	if captured != 0 {
		next.Count--
	}
	movedCode := codeFromInt8(moved)
	capturedCode := codeFromInt8(captured)
	movedColor := 0
	if moved < 0 {
		movedColor = 1
	}
	capturedColor := 0
	if captured < 0 {
		capturedColor = 1
	}
	var rebuild [2]bool
	for p := 0; p < 2; p++ {
		if movedColor == p && absI8(moved) == 1 {
			rebuild[p] = true
		}
		if captured != 0 && capturedColor == p && (absI8(captured) == 2 || absI8(captured) == 3) {
			rebuild[p] = true
		}
	}
	var newPieces [90]int
	if rebuild[0] || rebuild[1] {
		var newBoard [90]int8
		copy(newBoard[:], board[:])
		newBoard[from] = 0
		newBoard[to] = moved
		for i, p := range newBoard {
			newPieces[i] = codeFromInt8(p)
		}
	}
	for p := 0; p < 2; p++ {
		if rebuild[p] {
			n.rebuildPerspective(&next, newPieces[:], p)
			continue
		}
		var add, remove [3]int
		nAdd, nRemove := 0, 0
		remove[nRemove] = featureIndex(from, movedCode, acc.Kings[p], acc.AB[p], p)
		nRemove++
		if captured != 0 {
			remove[nRemove] = featureIndex(to, capturedCode, acc.Kings[p], acc.AB[p], p)
			nRemove++
		}
		add[nAdd] = featureIndex(to, movedCode, acc.Kings[p], acc.AB[p], p)
		nAdd++
		n.applyDelta(&next.Values[p], &next.PSQT[p], add[:nAdd], remove[:nRemove])
	}
	return &next
}

func absI8(v int8) int8 {
	if v < 0 {
		return -v
	}
	return v
}

func (n *Network) applyDelta(values *[ftHalf]int32, psqt *[psqtBuckets]int32, add, remove []int) {
	if len(add) == 1 && len(remove) == 1 {
		ab := add[0] * ftHalf
		rb := remove[0] * ftHalf
		if useAVX2 {
			applyDeltaAVX2(&values[0], &n.weights[ab], &n.weights[rb], ftHalf)
		} else {
			for j := 0; j < ftHalf; j++ {
				values[j] += int32(n.weights[ab+j]) - int32(n.weights[rb+j])
			}
		}
		ap := add[0] * psqtBuckets
		rp := remove[0] * psqtBuckets
		for j := 0; j < psqtBuckets; j++ {
			psqt[j] += n.psqt[ap+j] - n.psqt[rp+j]
		}
		return
	}
	for j := 0; j < ftHalf; j++ {
		d := int32(0)
		for _, idx := range add {
			d += int32(n.weights[idx*ftHalf+j])
		}
		for _, idx := range remove {
			d -= int32(n.weights[idx*ftHalf+j])
		}
		values[j] += d
	}
	for j := 0; j < psqtBuckets; j++ {
		d := int32(0)
		for _, idx := range add {
			d += n.psqt[idx*psqtBuckets+j]
		}
		for _, idx := range remove {
			d -= n.psqt[idx*psqtBuckets+j]
		}
		psqt[j] += d
	}
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
			a := int(acc.Values[p][j])
			if a < 0 {
				a = 0
			} else if a > 127 {
				a = 127
			}
			b := int(acc.Values[p][j+ftHalf/2])
			if b < 0 {
				b = 0
			} else if b > 127 {
				b = 127
			}
			transformed[offset+j] = uint8(a * b / 128)
		}
	}
	psqtEval := int32((acc.PSQT[side][bucket] - acc.PSQT[1-side][bucket]) / 2)
	positional := n.layers[bucket].propagate(transformed)
	return int((psqtEval + positional) / outputScale), nil
}
