package engine

import (
	"math"
	"runtime"
	"sort"
	"sync"
	"time"

	"minixiangqi/internal/minixiangqi"
)

const (
	scoreInf = 1_000_000_000
)

func (e *Engine) runAlphaBeta(pos *minixiangqi.Position, cfg SearchConfig) SearchResult {
	start := time.Now()
	e.nodes = 0
	if e.tt == nil {
		e.tt = make(map[uint64]ttEntry, 1<<16)
	}

	deadline := time.Time{}
	if cfg.TimeLimit > 0 {
		deadline = start.Add(cfg.TimeLimit)
	}

	bestMove := minixiangqi.Move{}
	bestScore := 0
	reachedDepth := 0
	var rootAcc *nnueAccumulator
	if e.nnue != nil {
		acc, err := e.nnue.NewAccumulator(pos)
		if err != nil {
			return SearchResult{
				TimeUsed: time.Since(start),
				Nodes:    e.nodes,
				NNFailed: true,
			}
		}
		rootAcc = &acc
	}

	for depth := 1; depth <= cfg.MaxDepth; depth++ {
		if !deadline.IsZero() && time.Now().After(deadline) {
			break
		}
		score, move, err := e.alphaBetaRoot(pos, depth, -scoreInf, scoreInf, deadline, rootAcc)
		if err != nil {
			return SearchResult{
				TimeUsed: time.Since(start),
				Nodes:    e.nodes,
				NNFailed: true,
			}
		}
		if move == (minixiangqi.Move{}) {
			break
		}
		bestMove = move
		bestScore = score
		reachedDepth = depth
	}

	if bestMove == (minixiangqi.Move{}) {
		status := minixiangqi.EvaluateStatus(pos)
		bestScore = terminalScore(status)
	}

	_ = reachedDepth
	return SearchResult{
		BestMove: bestMove,
		Score:    bestScore,
		WinProb:  scoreToRedWinProb(bestScore),
		Nodes:    e.nodes,
		TimeUsed: time.Since(start),
	}
}

func (e *Engine) alphaBetaRoot(pos *minixiangqi.Position, depth int, alpha, beta int, deadline time.Time, acc *nnueAccumulator) (int, minixiangqi.Move, error) {
	moves := pos.GenerateLegalMoves()
	moves = e.FilterVCFMoves(pos, moves)
	if len(moves) == 0 {
		return terminalScore(minixiangqi.EvaluateStatus(pos)), minixiangqi.Move{}, nil
	}

	if err := e.orderMoves(pos, moves); err != nil {
		return 0, minixiangqi.Move{}, err
	}

	key := ttKeyForPosition(pos)
	if entry, ok := e.tt[key]; ok {
		for i := range moves {
			if moves[i] == entry.Move {
				moves[0], moves[i] = moves[i], moves[0]
				break
			}
		}
	}
	orderMovesByTacticalPriority(pos, moves)

	bestMove := minixiangqi.Move{}
	bestScore := math.MinInt
	if pos.SideToMove == minixiangqi.Black {
		bestScore = math.MaxInt
	}
	workerCount := runtime.GOMAXPROCS(0)
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > len(moves) {
		workerCount = len(moves)
	}
	if depth <= 1 || workerCount <= 1 {
		for _, mv := range moves {
			if !deadline.IsZero() && time.Now().After(deadline) {
				break
			}
			child, ok := pos.ApplyMove(mv)
			if !ok {
				continue
			}
			childAcc, err := e.deriveAccumulator(pos, child, mv, acc)
			if err != nil {
				return 0, minixiangqi.Move{}, err
			}
			score, err := e.alphaBeta(child, depth-1, alpha, beta, deadline, childAcc)
			if err != nil {
				return 0, minixiangqi.Move{}, err
			}

			if pos.SideToMove == minixiangqi.Red {
				if score > bestScore {
					bestScore = score
					bestMove = mv
				}
				if score > alpha {
					alpha = score
				}
			} else {
				if score < bestScore {
					bestScore = score
					bestMove = mv
				}
				if score < beta {
					beta = score
				}
			}
			if alpha >= beta {
				break
			}
		}
		if bestMove != (minixiangqi.Move{}) {
			e.storeTT(key, depth, bestScore, ttExact, bestMove)
		}
		return bestScore, bestMove, nil
	}

	type rootTask struct {
		index int
		move  minixiangqi.Move
	}
	type rootResult struct {
		index int
		move  minixiangqi.Move
		score int
		nodes int64
		err   error
	}

	tasks := make(chan rootTask, len(moves))
	results := make(chan rootResult, len(moves))
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker := e.CloneForGame()
			for task := range tasks {
				if !deadline.IsZero() && time.Now().After(deadline) {
					results <- rootResult{index: task.index, move: task.move}
					continue
				}
				child, ok := pos.ApplyMove(task.move)
				if !ok {
					results <- rootResult{index: task.index, move: task.move}
					continue
				}
				childAcc, err := worker.deriveAccumulator(pos, child, task.move, acc)
				if err != nil {
					results <- rootResult{index: task.index, move: task.move, err: err}
					continue
				}
				score, err := worker.alphaBeta(child, depth-1, -scoreInf, scoreInf, deadline, childAcc)
				results <- rootResult{
					index: task.index,
					move:  task.move,
					score: score,
					nodes: worker.nodes,
					err:   err,
				}
				worker.nodes = 0
			}
		}()
	}
	for i, mv := range moves {
		tasks <- rootTask{index: i, move: mv}
	}
	close(tasks)
	wg.Wait()
	close(results)

	bestIndex := len(moves) + 1
	for result := range results {
		e.nodes += result.nodes
		if result.err != nil {
			return 0, minixiangqi.Move{}, result.err
		}
		if result.move == (minixiangqi.Move{}) {
			continue
		}
		if pos.SideToMove == minixiangqi.Red {
			if result.score > bestScore || (result.score == bestScore && result.index < bestIndex) {
				bestScore = result.score
				bestMove = result.move
				bestIndex = result.index
			}
		} else {
			if result.score < bestScore || (result.score == bestScore && result.index < bestIndex) {
				bestScore = result.score
				bestMove = result.move
				bestIndex = result.index
			}
		}
	}

	if bestMove != (minixiangqi.Move{}) {
		e.storeTT(key, depth, bestScore, ttExact, bestMove)
	}
	return bestScore, bestMove, nil
}

func (e *Engine) alphaBeta(pos *minixiangqi.Position, depth int, alpha, beta int, deadline time.Time, acc *nnueAccumulator) (int, error) {
	e.nodes++
	status := minixiangqi.EvaluateStatus(pos)
	if status != minixiangqi.StatusOngoing {
		return terminalScore(status), nil
	}
	if depth <= 0 {
		return e.evalWithAccumulator(pos, acc)
	}
	if !deadline.IsZero() && time.Now().After(deadline) {
		return e.evalWithAccumulator(pos, acc)
	}

	key := ttKeyForPosition(pos)
	origAlpha, origBeta := alpha, beta
	ttMove := minixiangqi.Move{}
	if entry, ok := e.tt[key]; ok {
		ttMove = entry.Move
		if entry.Depth >= depth {
			switch entry.Flag {
			case ttExact:
				return entry.Score, nil
			case ttUpperBound:
				if entry.Score <= alpha {
					return entry.Score, nil
				}
				if entry.Score < beta {
					beta = entry.Score
				}
			case ttLowerBound:
				if entry.Score >= beta {
					return entry.Score, nil
				}
				if entry.Score > alpha {
					alpha = entry.Score
				}
			}
			if alpha >= beta {
				return entry.Score, nil
			}
		}
	}

	moves := pos.GenerateLegalMoves()
	moves = e.FilterVCFMoves(pos, moves)
	if len(moves) == 0 {
		return terminalScore(minixiangqi.EvaluateStatus(pos)), nil
	}
	orderMovesByCaptureFirst(pos, moves)
	if ttMove != (minixiangqi.Move{}) {
		for i := range moves {
			if moves[i] == ttMove {
				moves[0], moves[i] = moves[i], moves[0]
				break
			}
		}
	}
	orderMovesByTacticalPriority(pos, moves)

	bestMove := minixiangqi.Move{}
	bestScore := math.MinInt
	if pos.SideToMove == minixiangqi.Black {
		bestScore = math.MaxInt
	}

	for _, mv := range moves {
		child, ok := pos.ApplyMove(mv)
		if !ok {
			continue
		}
		childAcc, err := e.deriveAccumulator(pos, child, mv, acc)
		if err != nil {
			return 0, err
		}
		score, err := e.alphaBeta(child, depth-1, alpha, beta, deadline, childAcc)
		if err != nil {
			return 0, err
		}
		if pos.SideToMove == minixiangqi.Red {
			if score > bestScore {
				bestScore = score
				bestMove = mv
			}
			if score > alpha {
				alpha = score
			}
		} else {
			if score < bestScore {
				bestScore = score
				bestMove = mv
			}
			if score < beta {
				beta = score
			}
		}
		if alpha >= beta {
			break
		}
	}

	if bestMove == (minixiangqi.Move{}) {
		return e.evalWithAccumulator(pos, acc)
	}

	flag := ttExact
	if bestScore <= origAlpha {
		flag = ttUpperBound
	} else if bestScore >= origBeta {
		flag = ttLowerBound
	}
	e.storeTT(key, depth, bestScore, flag, bestMove)
	return bestScore, nil
}

func (e *Engine) eval(pos *minixiangqi.Position) (int, error) {
	return e.evalWithAccumulator(pos, nil)
}

func (e *Engine) evalWithAccumulator(pos *minixiangqi.Position, acc *nnueAccumulator) (int, error) {
	status := minixiangqi.EvaluateStatus(pos)
	if status != minixiangqi.StatusOngoing {
		return terminalScore(status), nil
	}
	if e == nil {
		return 0, nil
	}
	if e.nnue != nil {
		return e.nnue.EvaluateWithAccumulator(pos, acc)
	}
	if e.nn == nil {
		return 0, nil
	}
	res, err := e.nn.Evaluate(pos)
	if err != nil {
		return 0, err
	}
	score := float64(res.WinProb - res.LossProb)
	if pos.SideToMove == minixiangqi.Black {
		score = float64(res.LossProb - res.WinProb)
	}
	return int(score * 10000), nil
}

func (e *Engine) deriveAccumulator(parent, child *minixiangqi.Position, mv minixiangqi.Move, acc *nnueAccumulator) (*nnueAccumulator, error) {
	if e == nil || e.nnue == nil {
		return nil, nil
	}
	next, err := e.nnue.DeriveAccumulator(parent, child, mv, acc)
	if err != nil {
		return nil, err
	}
	return &next, nil
}

func terminalScore(status minixiangqi.Status) int {
	switch status {
	case minixiangqi.StatusRedWin:
		return scoreInf
	case minixiangqi.StatusBlackWin:
		return -scoreInf
	case minixiangqi.StatusDraw:
		return 0
	default:
		return 0
	}
}

func scoreToRedWinProb(score int) float32 {
	if score >= scoreInf/2 {
		return 1
	}
	if score <= -scoreInf/2 {
		return 0
	}
	winProb := (float32(score)/10000.0 + 1.0) / 2.0
	if winProb < 0 {
		return 0
	}
	if winProb > 1 {
		return 1
	}
	return winProb
}

func (e *Engine) orderMoves(pos *minixiangqi.Position, moves []minixiangqi.Move) error {
	if len(moves) <= 1 {
		return nil
	}
	if e == nil || e.nn == nil {
		orderMovesByCaptureFirst(pos, moves)
		return nil
	}
	stage0, err := e.nn.EvaluateWithStage(pos, 0, -1)
	if err != nil {
		return err
	}
	stage1Cache := make(map[int]*NNResult)
	type moveScore struct {
		move  minixiangqi.Move
		score float32
	}
	scores := make([]moveScore, 0, len(moves))
	for _, mv := range moves {
		res1 := stage1Cache[mv.From]
		if res1 == nil {
			res1, err = e.nn.EvaluateWithStage(pos, 1, mv.From)
			if err != nil {
				return err
			}
			stage1Cache[mv.From] = res1
		}
		scores = append(scores, moveScore{
			move:  mv,
			score: max32(stage0.Policy[mv.From], 0) * max32(res1.Policy[mv.To], 0),
		})
	}
	sort.SliceStable(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})
	for i, item := range scores {
		moves[i] = item.move
	}
	return nil
}

func orderMovesByCaptureFirst(pos *minixiangqi.Position, moves []minixiangqi.Move) {
	sort.SliceStable(moves, func(i, j int) bool {
		return moveTacticalScore(pos, moves[i]) > moveTacticalScore(pos, moves[j])
	})
}

func orderMovesByTacticalPriority(pos *minixiangqi.Position, moves []minixiangqi.Move) {
	sort.SliceStable(moves, func(i, j int) bool {
		return moveTacticalScore(pos, moves[i]) > moveTacticalScore(pos, moves[j])
	})
}

func moveTacticalScore(pos *minixiangqi.Position, mv minixiangqi.Move) int {
	if pos == nil || mv.From < 0 || mv.From >= minixiangqi.NumSquares || mv.To < 0 || mv.To >= minixiangqi.NumSquares {
		return 0
	}
	score := 0
	moving := pos.Board[mv.From]
	target := pos.Board[mv.To]
	if target != minixiangqi.Empty {
		score += 200 + int(target.Type())*20
		if target.Type() == minixiangqi.PieceKing {
			score += 100000
		}
	}
	switch moving.Type() {
	case minixiangqi.PieceKing:
		score += 80
	case minixiangqi.PieceRook:
		score += 60
	case minixiangqi.PieceCannon:
		score += 50
	case minixiangqi.PieceHorse:
		score += 40
	case minixiangqi.PiecePawn:
		score += 20
	}
	return score
}
