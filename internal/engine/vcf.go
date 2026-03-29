package engine

import (
	"sort"

	"minixiangqi/internal/minixiangqi"
)

const (
	vcfDepthCap         = 24
	vcfDefaultDepth     = 8
	vcfDepthFilter      = 5
	vcfDepthRoot        = 6
	vcfNodeBudgetBase   = 32000
	vcfNodeBudgetPerPly = 8000
)

const (
	vcfModeAttack uint64 = 0xA5A5A5A5A5A5A5A5
	vcfModeDefend uint64 = 0x5A5A5A5A5A5A5A5A
)

type vcfTTEntry struct {
	Depth  int
	Result bool
	Move   minixiangqi.Move
}

type vcfContext struct {
	tt         map[uint64]vcfTTEntry
	inPath     map[uint64]bool
	nodes      int
	nodeBudget int
}

type VCFResult struct {
	CanWin bool
	Move   minixiangqi.Move
}

type scoredVCFMove struct {
	move  minixiangqi.Move
	score int
}

func (e *Engine) VCFSearch(pos *minixiangqi.Position, maxDepth int) VCFResult {
	if pos == nil {
		return VCFResult{}
	}
	if maxDepth <= 0 {
		maxDepth = vcfDefaultDepth
	}
	if maxDepth > vcfDepthCap {
		maxDepth = vcfDepthCap
	}

	ctx := &vcfContext{
		tt:         make(map[uint64]vcfTTEntry, 1<<16),
		inPath:     make(map[uint64]bool, 1<<10),
		nodeBudget: vcfNodeBudgetBase + maxDepth*vcfNodeBudgetPerPly,
	}

	var bestMove minixiangqi.Move
	for d := 2; d <= maxDepth; d += 2 {
		found, move := e.vcfRootSearch(pos, d, ctx)
		if found {
			return VCFResult{CanWin: true, Move: move}
		}
		bestMove = move
		if ctx.reachNodeBudget() {
			break
		}
	}
	return VCFResult{CanWin: false, Move: bestMove}
}

func (e *Engine) vcfRootSearch(pos *minixiangqi.Position, depth int, ctx *vcfContext) (bool, minixiangqi.Move) {
	moves := e.orderVCFMoves(pos, pos.GenerateLegalMoves(), ctx)
	for _, mv := range moves {
		nextPos, ok := pos.ApplyMove(mv)
		if !ok {
			continue
		}
		target := pos.Board[mv.To]
		if target != minixiangqi.Empty && target.Type() == minixiangqi.PieceKing {
			return true, mv
		}
		if !nextPos.IsInCheck(nextPos.SideToMove) {
			continue
		}
		if !e.vcfDefenderCanEscape(nextPos, depth-1, ctx) {
			return true, mv
		}
	}
	return false, minixiangqi.Move{}
}

func (e *Engine) orderVCFMoves(pos *minixiangqi.Position, moves []minixiangqi.Move, ctx *vcfContext) []minixiangqi.Move {
	scored := make([]scoredVCFMove, 0, len(moves))
	key := pos.EnsureHash() ^ vcfModeAttack
	ttMove := minixiangqi.Move{}
	if entry, ok := ctx.tt[key]; ok {
		ttMove = entry.Move
	}

	for _, mv := range moves {
		score := 0
		if mv == ttMove {
			score = 1000
		} else {
			moving := pos.Board[mv.From]
			target := pos.Board[mv.To]
			if target != minixiangqi.Empty {
				score += 100 + int(target.Type())
			}
			switch moving.Type() {
			case minixiangqi.PieceKing:
				score += 500
			case minixiangqi.PieceRook:
				score += 80
			case minixiangqi.PieceCannon:
				score += 60
			case minixiangqi.PieceHorse:
				score += 40
			case minixiangqi.PiecePawn:
				score += 20
			}
		}
		scored = append(scored, scoredVCFMove{move: mv, score: score})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	out := make([]minixiangqi.Move, len(scored))
	for i, item := range scored {
		out[i] = item.move
	}
	return out
}

func (e *Engine) vcfAttackerCanForce(pos *minixiangqi.Position, depth int, ctx *vcfContext) bool {
	if depth <= 0 || ctx.reachNodeBudget() {
		return false
	}
	key := pos.EnsureHash() ^ vcfModeAttack
	if ctx.inPath[key] {
		return false
	}
	if entry, ok := ctx.tt[key]; ok && entry.Depth >= depth {
		return entry.Result
	}
	ctx.inPath[key] = true
	defer delete(ctx.inPath, key)

	moves := e.orderVCFMoves(pos, pos.GenerateLegalMoves(), ctx)
	result := false
	bestMove := minixiangqi.Move{}
	for _, mv := range moves {
		nextPos, ok := pos.ApplyMove(mv)
		if !ok {
			continue
		}
		target := pos.Board[mv.To]
		if target != minixiangqi.Empty && target.Type() == minixiangqi.PieceKing {
			result = true
			bestMove = mv
			break
		}
		if !nextPos.IsInCheck(nextPos.SideToMove) {
			continue
		}
		if !e.vcfDefenderCanEscape(nextPos, depth-1, ctx) {
			result = true
			bestMove = mv
			break
		}
	}
	ctx.tt[key] = vcfTTEntry{Depth: depth, Result: result, Move: bestMove}
	return result
}

func (e *Engine) vcfDefenderCanEscape(pos *minixiangqi.Position, depth int, ctx *vcfContext) bool {
	if depth <= 0 || ctx.reachNodeBudget() {
		return true
	}
	key := pos.EnsureHash() ^ vcfModeDefend
	if ctx.inPath[key] {
		return true
	}
	if entry, ok := ctx.tt[key]; ok && entry.Depth >= depth {
		return entry.Result
	}
	ctx.inPath[key] = true
	defer delete(ctx.inPath, key)

	moves := pos.GenerateLegalMoves()
	if len(moves) == 0 {
		ctx.tt[key] = vcfTTEntry{Depth: depth, Result: false}
		return false
	}

	result := false
	bestMove := minixiangqi.Move{}
	for _, mv := range moves {
		nextPos, ok := pos.ApplyMove(mv)
		if !ok {
			continue
		}
		if !e.vcfAttackerCanForce(nextPos, depth-1, ctx) {
			result = true
			bestMove = mv
			break
		}
	}
	ctx.tt[key] = vcfTTEntry{Depth: depth, Result: result, Move: bestMove}
	return result
}

func (ctx *vcfContext) reachNodeBudget() bool {
	ctx.nodes++
	return ctx.nodes > ctx.nodeBudget
}

func (e *Engine) CanCaptureKingNext(pos *minixiangqi.Position) bool {
	if pos == nil {
		return false
	}
	for _, mv := range pos.GenerateLegalMoves() {
		target := pos.Board[mv.To]
		if target != minixiangqi.Empty && target.Type() == minixiangqi.PieceKing {
			return true
		}
	}
	return false
}

func (e *Engine) FilterVCFMoves(pos *minixiangqi.Position, moves []minixiangqi.Move) []minixiangqi.Move {
	if pos == nil || len(moves) <= 1 {
		return moves
	}
	safeMoves := make([]minixiangqi.Move, 0, len(moves))
	for _, mv := range moves {
		target := pos.Board[mv.To]
		if target != minixiangqi.Empty && target.Type() == minixiangqi.PieceKing {
			safeMoves = append(safeMoves, mv)
			continue
		}
		nextPos, ok := pos.ApplyMove(mv)
		if !ok {
			continue
		}
		if e.CanCaptureKingNext(nextPos) {
			continue
		}
		if e.VCFSearch(nextPos, vcfDepthFilter).CanWin {
			continue
		}
		safeMoves = append(safeMoves, mv)
	}
	if len(safeMoves) == 0 {
		return moves
	}
	return safeMoves
}
