package xiangqi

import "math/bits"

// bitboard is a 90-bit set. Bit 0 is a0, bit 89 is i9.
type bitboard struct {
	lo, hi uint64
}

func bitOf(sq int) bitboard {
	if sq < 64 {
		return bitboard{lo: 1 << uint(sq)}
	}
	return bitboard{hi: 1 << uint(sq-64)}
}

func (b bitboard) isZero() bool { return b.lo == 0 && b.hi == 0 }
func (b bitboard) has(sq int) bool {
	if sq < 64 {
		return b.lo&(1<<uint(sq)) != 0
	}
	return b.hi&(1<<uint(sq-64)) != 0
}
func (b *bitboard) set(sq int) {
	if sq < 64 {
		b.lo |= 1 << uint(sq)
	} else {
		b.hi |= 1 << uint(sq-64)
	}
}
func (b *bitboard) clear(sq int) {
	if sq < 64 {
		b.lo &^= 1 << uint(sq)
	} else {
		b.hi &^= 1 << uint(sq-64)
	}
}
func (b bitboard) or(o bitboard) bitboard  { return bitboard{b.lo | o.lo, b.hi | o.hi} }
func (b bitboard) and(o bitboard) bitboard { return bitboard{b.lo & o.lo, b.hi & o.hi} }
func (b bitboard) andNot(o bitboard) bitboard {
	return bitboard{b.lo &^ o.lo, b.hi &^ o.hi}
}
func (b bitboard) lsb() int {
	if b.lo != 0 {
		return bits.TrailingZeros64(b.lo)
	}
	if b.hi != 0 {
		return 64 + bits.TrailingZeros64(b.hi)
	}
	return -1
}
func (b bitboard) msb() int {
	if b.hi != 0 {
		return 64 + bits.Len64(b.hi) - 1
	}
	if b.lo != 0 {
		return bits.Len64(b.lo) - 1
	}
	return -1
}

var (
	rayN, rayE, rayS, rayW [Size]bitboard
	rays                   [4][Size]bitboard
	palace                 [2]bitboard
	ownSide                [2]bitboard
	kingTargets            [Size]bitboard
	advisorTargets         [Size]bitboard
	bishopTargets          [Size]bitboard
	knightTargets          [Size]bitboard
	pawnTargets            [2][Size]bitboard
	pawnAttackers          [2][Size]bitboard
	knightAttackers        [Size]bitboard
)

func init() {
	for sq := 0; sq < Size; sq++ {
		file, rank := sq%Files, sq/Files
		for r := rank + 1; r < Ranks; r++ {
			rayN[sq].set(r*Files + file)
		}
		for f := file + 1; f < Files; f++ {
			rayE[sq].set(rank*Files + f)
		}
		for r := rank - 1; r >= 0; r-- {
			rayS[sq].set(r*Files + file)
		}
		for f := file - 1; f >= 0; f-- {
			rayW[sq].set(rank*Files + f)
		}
		for _, d := range orthogonal {
			if inBoard(file+d[0], rank+d[1]) {
				kingTargets[sq].set((rank+d[1])*Files + file + d[0])
			}
		}
		for _, d := range diagonal {
			if inBoard(file+d[0], rank+d[1]) {
				advisorTargets[sq].set((rank+d[1])*Files + file + d[0])
			}
			if inBoard(file+2*d[0], rank+2*d[1]) {
				bishopTargets[sq].set((rank+2*d[1])*Files + file + 2*d[0])
			}
		}
		for _, d := range knightMoves {
			f, r := file+d[0], rank+d[1]
			if inBoard(f, r) {
				knightTargets[sq].set(r*Files + f)
			}
		}
		if inPalace(file, rank, Red) {
			palace[0].set(sq)
		}
		if inPalace(file, rank, Black) {
			palace[1].set(sq)
		}
		if onOwnSide(rank, Red) {
			ownSide[0].set(sq)
		}
		if onOwnSide(rank, Black) {
			ownSide[1].set(sq)
		}
		rays[0][sq], rays[1][sq], rays[2][sq], rays[3][sq] = rayN[sq], rayE[sq], rayS[sq], rayW[sq]
	}
	for side := 0; side < 2; side++ {
		for sq := 0; sq < Size; sq++ {
			file, rank := sq%Files, sq/Files
			dir := 1
			if side == 1 {
				dir = -1
			}
			if r := rank + dir; inBoard(file, r) {
				pawnTargets[side][sq].set(r*Files + file)
				pawnAttackers[side][r*Files+file].set(sq)
			}
			crossed := (side == 0 && rank >= 5) || (side == 1 && rank <= 4)
			if crossed {
				for _, df := range []int{-1, 1} {
					if f := file + df; inBoard(f, rank) {
						pawnTargets[side][sq].set(rank*Files + f)
						pawnAttackers[side][rank*Files+f].set(sq)
					}
				}
			}
		}
	}
	for sq := 0; sq < Size; sq++ {
		for from := 0; from < Size; from++ {
			if knightTargets[from].has(sq) {
				knightAttackers[sq].set(from)
			}
		}
	}
}

type Position struct {
	Board    [Size]int8
	Turn     int8
	Halfmove int
	Fullmove int
	hash     uint64
	occ      [2]bitboard
	pieces   [16]bitboard
}

type undoMove struct {
	from, to       int
	piece, capture int8
	halfmove       int
	fullmove       int
	turn           int8
	hash           uint64
}

func newPosition(st *State) *Position {
	p := &Position{Board: st.Board, Turn: st.Turn, Halfmove: st.Halfmove, Fullmove: st.Fullmove, hash: st.hash}
	p.rebuild()
	return p
}

func (p *Position) rebuild() {
	p.occ[0], p.occ[1] = bitboard{}, bitboard{}
	for i := range p.pieces {
		p.pieces[i] = bitboard{}
	}
	for sq, pc := range p.Board {
		if pc == 0 {
			continue
		}
		p.pieces[int(pc)+8].set(sq)
		if pc > 0 {
			p.occ[0].set(sq)
		} else {
			p.occ[1].set(sq)
		}
	}
	if p.hash == 0 {
		p.hash = calcHash(p.Board, p.Turn)
	}
}

func (p *Position) clone() *Position {
	q := *p
	return &q
}

func (p *Position) doMove(m move) undoMove {
	u := undoMove{from: m.From, to: m.To, piece: p.Board[m.From], capture: p.Board[m.To], halfmove: p.Halfmove, fullmove: p.Fullmove, turn: p.Turn, hash: p.hash}
	pc := u.piece
	p.Board[m.From] = 0
	p.Board[m.To] = pc
	p.pieces[int(pc)+8].clear(m.From)
	p.pieces[int(pc)+8].set(m.To)
	if pc > 0 {
		p.occ[0].clear(m.From)
		p.occ[0].set(m.To)
	} else {
		p.occ[1].clear(m.From)
		p.occ[1].set(m.To)
	}
	if u.capture != 0 {
		cap := u.capture
		p.pieces[int(cap)+8].clear(m.To)
		if cap > 0 {
			p.occ[0].clear(m.To)
		} else {
			p.occ[1].clear(m.To)
		}
		p.Halfmove = 0
	} else if absPiece(pc) == Pawn {
		p.Halfmove = 0
	} else {
		p.Halfmove++
	}
	if p.Turn == Black {
		p.Fullmove++
	}
	p.hash ^= zobristPieces[int(pc)+8][m.From] ^ zobristPieces[int(pc)+8][m.To]
	if u.capture != 0 {
		p.hash ^= zobristPieces[int(u.capture)+8][m.To]
	}
	p.hash ^= zobristTurn
	p.Turn = -p.Turn
	return u
}

func (p *Position) undo(u undoMove) {
	p.Board[u.from], p.Board[u.to] = u.piece, u.capture
	p.pieces[int(u.piece)+8].set(u.from)
	p.pieces[int(u.piece)+8].clear(u.to)
	if u.piece > 0 {
		p.occ[0].set(u.from)
		p.occ[0].clear(u.to)
	} else {
		p.occ[1].set(u.from)
		p.occ[1].clear(u.to)
	}
	if u.capture != 0 {
		p.pieces[int(u.capture)+8].set(u.to)
		if u.capture > 0 {
			p.occ[0].set(u.to)
		} else {
			p.occ[1].set(u.to)
		}
	}
	p.Halfmove, p.Fullmove, p.Turn, p.hash = u.halfmove, u.fullmove, u.turn, u.hash
}

func (p *Position) key() uint64 { return p.hash }

func nearest(mask bitboard, east bool) int {
	if east {
		return mask.lsb()
	}
	return mask.msb()
}

func rookAttacks(sq int, occ bitboard) bitboard {
	var out bitboard
	for d := 0; d < 4; d++ {
		r := rays[d][sq]
		blockers := r.and(occ)
		if blockers.isZero() {
			out = out.or(r)
		} else {
			block := nearest(blockers, d == 0 || d == 1)
			out = out.or(r.andNot(rays[d][block]))
		}
	}
	return out
}

func cannonTargets(sq int, occ, enemy bitboard) bitboard {
	var out bitboard
	for d := 0; d < 4; d++ {
		r := rays[d][sq]
		blockers := r.and(occ)
		if blockers.isZero() {
			out = out.or(r)
			continue
		}
		first := nearest(blockers, d == 0 || d == 1)
		before := r.andNot(rays[d][first])
		out = out.or(before.andNot(bitOf(first)))
		secondMask := rays[d][first].and(occ)
		if secondMask.isZero() {
			continue
		}
		second := nearest(secondMask, d == 0 || d == 1)
		if enemy.has(second) {
			out.set(second)
		}
	}
	return out
}

func knightLeg(from, to int) int {
	ff, fr := from%Files, from/Files
	tf, tr := to%Files, to/Files
	if abs(tf-ff) == 2 {
		return fr*Files + ff + sign(tf-ff)
	}
	return (fr+sign(tr-fr))*Files + ff
}

func bishopEye(from, to int) int {
	return ((from/Files+to/Files)/2)*Files + (from%Files+to%Files)/2
}

func (p *Position) appendTargets(from int, mask, own bitboard, out []move) []move {
	mask = mask.andNot(own)
	for !mask.isZero() {
		to := mask.lsb()
		mask.clear(to)
		out = append(out, move{from, to})
	}
	return out
}

func (p *Position) pseudoMoves() []move {
	side := 0
	if p.Turn == Black {
		side = 1
	}
	own := p.occ[side]
	enemy := p.occ[1-side]
	occ := own.or(enemy)
	out := make([]move, 0, 64)
	for bb := own; !bb.isZero(); {
		from := bb.lsb()
		bb.clear(from)
		pc := p.Board[from]
		switch absPiece(pc) {
		case King:
			out = p.appendTargets(from, kingTargets[from].and(palace[side]), own, out)
			for _, d := range []int{0, 2} {
				blockers := rays[d][from].and(occ)
				if !blockers.isZero() {
					block := nearest(blockers, d == 0)
					if p.Board[block] == -King*p.Turn {
						out = append(out, move{from, block})
					}
				}
			}
		case Advisor:
			out = p.appendTargets(from, advisorTargets[from].and(palace[side]), own, out)
		case Bishop:
			targets := bishopTargets[from].and(ownSide[side]).andNot(own)
			for !targets.isZero() {
				to := targets.lsb()
				targets.clear(to)
				if p.Board[bishopEye(from, to)] == 0 {
					out = append(out, move{from, to})
				}
			}
		case Knight:
			targets := knightTargets[from].andNot(own)
			for !targets.isZero() {
				to := targets.lsb()
				targets.clear(to)
				if p.Board[knightLeg(from, to)] == 0 {
					out = append(out, move{from, to})
				}
			}
		case Rook:
			out = p.appendTargets(from, rookAttacks(from, occ), own, out)
		case Cannon:
			out = p.appendTargets(from, cannonTargets(from, occ, enemy), own, out)
		case Pawn:
			out = p.appendTargets(from, pawnTargets[side][from], own, out)
		}
	}
	return out
}

func (p *Position) isAttacked(sq int, by int8) bool {
	enemy := -by
	enemySide := 0
	if enemy == Black {
		enemySide = 1
	}
	occ := p.occ[0].or(p.occ[1])
	for d := 0; d < 4; d++ {
		blockers := rays[d][sq].and(occ)
		if blockers.isZero() {
			continue
		}
		first := nearest(blockers, d == 0 || d == 1)
		pc := p.Board[first]
		if pc != 0 && colorOf(pc) == enemy {
			t := absPiece(pc)
			if t == Rook || t == King {
				return true
			}
		}
		secondMask := rays[d][first].and(occ)
		if secondMask.isZero() {
			continue
		}
		second := nearest(secondMask, d == 0 || d == 1)
		if pc := p.Board[second]; pc != 0 && colorOf(pc) == enemy && absPiece(pc) == Cannon {
			return true
		}
	}
	knights := knightAttackers[sq].and(p.pieces[8-int(Knight)]).and(p.occ[enemySide])
	if enemy == Red {
		knights = knightAttackers[sq].and(p.pieces[int(Knight)+8]).and(p.occ[0])
	}
	for !knights.isZero() {
		from := knights.lsb()
		knights.clear(from)
		if p.Board[knightLeg(from, sq)] == 0 {
			return true
		}
	}
	pawnIdx := 8 - int(Pawn)
	if enemy == Red {
		pawnIdx = int(Pawn) + 8
	}
	return !pawnAttackers[enemySide][sq].and(p.pieces[pawnIdx]).and(p.occ[enemySide]).isZero()
}

func (p *Position) inCheck(side int8) bool {
	idx := 8 - int(King)
	if side == Red {
		idx = int(King) + 8
	}
	king := p.pieces[idx].lsb()
	if king < 0 {
		return true
	}
	return p.isAttacked(king, side)
}

func (p *Position) legalMoves() []move {
	pseudo := p.pseudoMoves()
	side := p.Turn
	out := make([]move, 0, len(pseudo))
	for _, m := range pseudo {
		u := p.doMove(m)
		if !p.inCheck(side) {
			out = append(out, m)
		}
		p.undo(u)
	}
	return out
}
