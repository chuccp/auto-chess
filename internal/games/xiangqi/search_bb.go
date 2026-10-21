package xiangqi

import (
	"context"
	"sync"
	"time"

	"github.com/chuccp/auto-chess/internal/game"
	"github.com/chuccp/auto-chess/internal/games/xiangqi/nnue"
)

func newSearchTables() *searchTables {
	return &searchTables{eval: make(map[uint64]int), tt: make(map[uint64]ttEntry)}
}

func evaluateBoard(board [Size]int8, turn int8) int {
	score := 0
	for idx, p := range board {
		if p == 0 {
			continue
		}
		piece := absPiece(p)
		value := pieceValue[piece]
		mirror := idx
		if p < 0 {
			mirror = (9-idx/9)*9 + (8 - idx%9)
		}
		value += evalTable(piece)[mirror]
		if p > 0 {
			score += value
		} else {
			score -= value
		}
	}
	if turn == Black {
		return -score
	}
	return score
}

func hasNonPawnMaterialPos(p *Position, side int8) bool {
	for _, pc := range p.Board {
		if pc == 0 || colorOf(pc) != side {
			continue
		}
		switch absPiece(pc) {
		case Rook, Cannon, Knight:
			return true
		}
	}
	return false
}

func (g *Game) evaluatePos(p *Position, acc *nnue.Accumulator, tables *searchTables) int {
	if g.nnue == nil || acc == nil {
		return evaluateBoard(p.Board, p.Turn)
	}
	key := p.key()
	if tables != nil {
		if score, ok := tables.eval[key]; ok {
			return score
		}
	}
	score, err := g.nnue.EvaluateAccumulator(acc, colorString(p.Turn))
	if err != nil {
		return evaluateBoard(p.Board, p.Turn)
	}
	if tables != nil {
		tables.eval[key] = score
	}
	return score
}

func (g *Game) searchPosition(ctx context.Context, pos game.Position, opts game.SearchOptions) (game.SearchResult, error) {
	st := pos.(*State)
	result := game.SearchResult{Game: "xiangqi", Difficulty: game.NormalizeDifficulty(opts.Difficulty)}
	if g.nnue != nil {
		result.Engine = "pikafish-nnue"
	}
	if status := g.status(st); status.Finished() {
		result.GameOver = true
		result.Status = status
		return result, nil
	}
	start := time.Now()
	root := newPosition(st)
	rootMoves := root.legalMoves()
	if len(rootMoves) == 0 {
		result.GameOver = true
		result.Status = g.status(st)
		return result, nil
	}
	depth := game.DepthFor(opts.Difficulty, opts.MaxDepth, 7)
	if opts.MaxDepth == 0 {
		switch game.NormalizeDifficulty(opts.Difficulty) {
		case "easy":
			depth = 2
		case "normal":
			depth = 4
		case "hard":
			depth = 5
		case "master":
			depth = 6
		}
	}
	limit := game.TimeFor(opts.Difficulty, opts.TimeLimit)
	deadline := start.Add(limit)
	nodes := int64(0)
	timedOut := false
	var rootAcc *nnue.Accumulator
	if g.nnue != nil {
		rootAcc, _ = g.nnue.NewAccumulator(nnuePieces(root.Board))
	}
	tables := newSearchTables()
	orderMovesBoard(root.Board, root.Turn, rootMoves, &tables.history, move{}, move{})
	best := rootMoves[0]
	bestScore := 0
	reached := 0
	for d := 1; d <= depth; d++ {
		if time.Now().After(deadline) || ctx.Err() != nil {
			timedOut = true
			break
		}
		alpha, beta := -mateScore*2, mateScore*2
		localBest := rootMoves[0]
		localScore := 0
		complete := true
		if g.workers > 1 && d >= 3 && len(rootMoves) > 1 {
			var pn int64
			localBest, localScore, pn, complete = g.searchRootSplitPos(rootMoves, root, rootAcc, d, deadline, ctx)
			nodes += pn
			if !complete {
				timedOut = true
				break
			}
		} else {
			for i, m := range rootMoves {
				cp := root.clone()
				childAcc := nextAccumulator(g.nnue, rootAcc, root.Board, m)
				cp.doMove(m)
				score := -g.negamaxPos(cp, d-1, -beta, -alpha, 1, &nodes, deadline, ctx, &timedOut, childAcc, tables)
				if timedOut {
					complete = false
					break
				}
				if i == 0 || score > localScore {
					localBest = m
					localScore = score
				}
				if score > alpha {
					alpha = score
				}
			}
		}
		if !complete {
			break
		}
		best, bestScore, reached = localBest, localScore, d
		bestView := g.moveView(best)
		opts.Report(game.SearchProgress{
			Depth:  d,
			Score:  bestScore,
			Nodes:  nodes,
			TimeMs: time.Since(start).Milliseconds(),
			Best:   &bestView,
			PV:     []game.Move{bestView},
		})
		rootMoves = moveToFront(rootMoves, best)
		if abs(bestScore) >= mateScore-1000 {
			break
		}
	}
	next := makeMove(st, best)
	next.LastFrom, next.LastTo = best.From, best.To
	mv := g.moveView(best)
	mv.Check = inCheck(next, next.Turn)
	nv := g.Encode(next, false)
	result.Move = &mv
	result.Status = nv.Status
	result.GameOver = nv.Status.Finished()
	result.Position = &nv
	result.Evaluation = game.Evaluation{Score: bestScore, Unit: "centipawn", Perspective: colorString(st.Turn)}
	if abs(bestScore) >= mateScore-1000 {
		result.Evaluation.MateIn = (mateScore - abs(bestScore) + 1) / 2
		if bestScore < 0 {
			result.Evaluation.MateIn = -result.Evaluation.MateIn
		}
	}
	result.Search = game.SearchInfo{Depth: reached, Nodes: nodes, TimeMs: time.Since(start).Milliseconds(), TimedOut: timedOut, PV: []game.Move{mv}}
	return result, nil
}

func (g *Game) searchRootSplitPos(rootMoves []move, root *Position, rootAcc *nnue.Accumulator, depth int, deadline time.Time, ctx context.Context) (move, int, int64, bool) {
	first := rootMoves[0]
	cp := root.clone()
	childAcc := nextAccumulator(g.nnue, rootAcc, root.Board, first)
	cp.doMove(first)
	firstTables := newSearchTables()
	var nodes int64
	var timed bool
	bestScore := -g.negamaxPos(cp, depth-1, -mateScore*2, mateScore*2, 1, &nodes, deadline, ctx, &timed, childAcc, firstTables)
	if timed || time.Now().After(deadline) || ctx.Err() != nil {
		return first, bestScore, nodes, false
	}
	best := first
	alpha := bestScore
	type result struct {
		move  move
		score int
		nodes int64
		done  bool
	}
	results := make([]result, len(rootMoves))
	results[0] = result{move: first, score: bestScore, nodes: nodes, done: true}
	jobs := make(chan int)
	workers := g.workers
	if workers > len(rootMoves)-1 {
		workers = len(rootMoves) - 1
	}
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tables := newSearchTables()
			for idx := range jobs {
				r := result{move: rootMoves[idx], done: true}
				cp := root.clone()
				acc := nextAccumulator(g.nnue, rootAcc, root.Board, rootMoves[idx])
				cp.doMove(rootMoves[idx])
				var localTimed bool
				if time.Now().After(deadline) || ctx.Err() != nil {
					r.done = false
				} else {
					r.score = -g.negamaxPos(cp, depth-1, -mateScore*2, -alpha, 1, &r.nodes, deadline, ctx, &localTimed, acc, tables)
					if localTimed {
						r.done = false
					}
				}
				results[idx] = r
			}
		}()
	}
	for i := 1; i < len(rootMoves); i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	complete := true
	for i := 1; i < len(results); i++ {
		r := results[i]
		nodes += r.nodes
		if !r.done {
			complete = false
			continue
		}
		if r.score > bestScore {
			best = r.move
			bestScore = r.score
		}
	}
	if !complete {
		return best, bestScore, nodes, false
	}
	return best, bestScore, nodes, true
}

func (g *Game) negamaxPos(p *Position, depth, alpha, beta, ply int, nodes *int64, deadline time.Time, ctx context.Context, timedOut *bool, acc *nnue.Accumulator, tables *searchTables) int {
	*nodes++
	key := p.key()
	alpha0 := alpha
	if score, ok := tables.probe(key, depth, alpha, beta); ok {
		return score
	}
	if *nodes&1023 == 0 && (time.Now().After(deadline) || ctx.Err() != nil) {
		*timedOut = true
		return 0
	}
	if p.Halfmove >= 120 {
		return 0
	}
	moves := p.legalMoves()
	if len(moves) == 0 {
		return -mateScore + ply
	}
	if depth <= 0 {
		return g.quiescePos(p, alpha, beta, ply, nodes, deadline, ctx, timedOut, acc, tables)
	}
	if entry, ok := tables.tt[key]; ok && entry.best != (move{}) {
		moves = moveToFront(moves, entry.best)
	}
	orderMovesBoard(p.Board, p.Turn, moves, &tables.history, tables.killers[ply][0], tables.killers[ply][1])
	futility := false
	futilityScore := 0
	if depth == 1 && !p.inCheck(p.Turn) {
		futilityScore = g.evaluatePos(p, acc, tables)
		futility = futilityScore+150 <= alpha
	}
	if depth >= 3 && !p.inCheck(p.Turn) && hasNonPawnMaterialPos(p, p.Turn) {
		nullP := *p
		nullP.Turn = -p.Turn
		nullP.hash ^= zobristTurn
		nullP.Halfmove++
		score := -g.negamaxPos(&nullP, depth-3, -beta, -beta+1, ply+1, nodes, deadline, ctx, timedOut, acc, tables)
		if *timedOut {
			return 0
		}
		if score >= beta {
			return beta
		}
	}
	best := -mateScore * 2
	bestMove := move{}
	side := 0
	if p.Turn == Black {
		side = 1
	}
	for i, m := range moves {
		if futility && p.Board[m.To] == 0 {
			continue
		}
		childAcc := nextAccumulator(g.nnue, acc, p.Board, m)
		u := p.doMove(m)
		reduction := 0
		if depth >= 3 && i >= 3 && u.capture == 0 {
			reduction = 1
			if depth >= 5 && i >= 6 {
				reduction = 2
			}
		}
		score := -g.negamaxPos(p, depth-1-reduction, -beta, -alpha, ply+1, nodes, deadline, ctx, timedOut, childAcc, tables)
		if reduction > 0 && score > alpha && !*timedOut {
			score = -g.negamaxPos(p, depth-1, -beta, -alpha, ply+1, nodes, deadline, ctx, timedOut, childAcc, tables)
		}
		p.undo(u)
		if score > best {
			best = score
			bestMove = m
		}
		if score > alpha {
			alpha = score
		}
		if alpha >= beta {
			if u.capture == 0 {
				tables.history[side][m.From][m.To] += int32(depth * depth)
				if ply < len(tables.killers) && tables.killers[ply][0] != m {
					tables.killers[ply][1] = tables.killers[ply][0]
					tables.killers[ply][0] = m
				}
			}
			break
		}
		if *timedOut {
			break
		}
	}
	if futility && best < futilityScore {
		best = futilityScore
	}
	if !*timedOut {
		tables.store(key, depth, best, alpha0, beta, bestMove)
	}
	return best
}

func (g *Game) quiescePos(p *Position, alpha, beta, ply int, nodes *int64, deadline time.Time, ctx context.Context, timedOut *bool, acc *nnue.Accumulator, tables *searchTables) int {
	*nodes++
	if *nodes&1023 == 0 && (time.Now().After(deadline) || ctx.Err() != nil) {
		*timedOut = true
		return 0
	}
	if p.Halfmove >= 120 || ply >= 24 {
		return g.evaluatePos(p, acc, tables)
	}
	check := p.inCheck(p.Turn)
	stand := 0
	var moves []move
	if check {
		moves = p.legalMoves()
		if len(moves) == 0 {
			return -mateScore + ply
		}
	} else {
		stand = g.evaluatePos(p, acc, tables)
		if stand >= beta {
			return stand
		}
		if stand > alpha {
			alpha = stand
		}
		moves = p.legalMoves()
		captures := moves[:0]
		for _, m := range moves {
			if p.Board[m.To] != 0 {
				captures = append(captures, m)
			}
		}
		moves = captures
		if len(moves) == 0 {
			return alpha
		}
	}
	orderMovesBoard(p.Board, p.Turn, moves, &tables.history, tables.killers[ply][0], tables.killers[ply][1])
	best := -mateScore * 2
	side := 0
	if p.Turn == Black {
		side = 1
	}
	for _, m := range moves {
		if !check && p.Board[m.To] != 0 && stand+pieceValue[absPiece(p.Board[m.To])]+200 <= alpha {
			continue
		}
		childAcc := nextAccumulator(g.nnue, acc, p.Board, m)
		u := p.doMove(m)
		score := -g.quiescePos(p, -beta, -alpha, ply+1, nodes, deadline, ctx, timedOut, childAcc, tables)
		p.undo(u)
		if *timedOut {
			break
		}
		if score > best {
			best = score
		}
		if score > alpha {
			alpha = score
		}
		if alpha >= beta {
			if u.capture == 0 {
				tables.history[side][m.From][m.To]++
			}
			break
		}
	}
	if best == -mateScore*2 {
		return alpha
	}
	return best
}
