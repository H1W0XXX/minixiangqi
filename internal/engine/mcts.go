package engine

import (
	"fmt"
	"time"

	"minixiangqi/internal/minixiangqi"
)

type mctsNode struct {
	move     minixiangqi.Move
	prior    float64
	visits   int
	valueSum float64
	children []*mctsNode
	expanded bool
}

func (n *mctsNode) value() float64 {
	if n.visits == 0 {
		return 0
	}
	return n.valueSum / float64(n.visits)
}

func (e *Engine) runMCTS(pos *minixiangqi.Position, cfg SearchConfig) SearchResult {
	start := time.Now()
	root := &mctsNode{}
	var nnFailed bool

	children, err := e.expandRootNode(pos)
	if err != nil {
		nnFailed = true
	}
	root.children = children
	root.expanded = true
	if nnFailed || len(root.children) == 0 {
		return SearchResult{NNFailed: nnFailed, TimeUsed: time.Since(start)}
	}

	var nodes int64
	deadline := start.Add(cfg.TimeLimit)
	for sim := 0; sim < cfg.MCTSSimulations; sim++ {
		if time.Now().After(deadline) {
			break
		}
		if err := e.playout(root, pos.Clone(), &nodes); err != nil {
			nnFailed = true
			break
		}
	}

	best := root.children[0]
	for _, child := range root.children[1:] {
		if child.visits > best.visits {
			best = child
		}
	}

	redWinProb := float32(0.5)
	if e.nn != nil {
		rootValue, _ := e.nn.Evaluate(pos)
		if rootValue != nil {
			if pos.SideToMove == minixiangqi.Red {
				redWinProb = rootValue.WinProb
			} else {
				redWinProb = rootValue.LossProb
			}
		}
	}

	return SearchResult{
		BestMove: best.move,
		Score:    int(best.value() * 10000),
		WinProb:  redWinProb,
		Nodes:    nodes,
		TimeUsed: time.Since(start),
		NNFailed: nnFailed,
	}
}

func (e *Engine) playout(root *mctsNode, pos *minixiangqi.Position, nodes *int64) error {
	path := []*mctsNode{root}
	node := root

	for node.expanded && len(node.children) > 0 {
		best := node.children[0]
		bestScore := -1e18
		parentVisits := float64(node.visits + 1)
		for _, child := range node.children {
			q := 0.0
			if child.visits > 0 {
				q = -child.value()
			}
			u := mctsCpuct * child.prior * sqrt(parentVisits) / float64(1+child.visits)
			score := q + u
			if score > bestScore {
				bestScore = score
				best = child
			}
		}
		next, ok := pos.ApplyMove(best.move)
		if !ok {
			return fmt.Errorf("mcts encountered illegal move")
		}
		pos = next
		node = best
		path = append(path, node)
	}

	value, err := e.evaluateLeaf(node, pos)
	if err != nil {
		return err
	}

	for i := len(path) - 1; i >= 0; i-- {
		path[i].visits++
		path[i].valueSum += value
		value = -value
	}
	*nodes += 1
	return nil
}

func (e *Engine) evaluateLeaf(node *mctsNode, pos *minixiangqi.Position) (float64, error) {
	status := minixiangqi.EvaluateStatus(pos)
	switch status {
	case minixiangqi.StatusDraw:
		node.expanded = true
		return 0, nil
	case minixiangqi.StatusRedWin, minixiangqi.StatusBlackWin:
		node.expanded = true
		return -1, nil
	}

	children, err := e.expandNode(pos)
	if err != nil {
		return 0, err
	}
	node.children = children
	node.expanded = true
	if len(children) == 0 {
		return -1, nil
	}
	if e.nn == nil {
		return 0, nil
	}
	res, err := e.nn.Evaluate(pos)
	if err != nil {
		return 0, err
	}
	return float64(res.WinProb - res.LossProb), nil
}

func (e *Engine) expandNode(pos *minixiangqi.Position) ([]*mctsNode, error) {
	return e.expandMoves(pos, pos.GenerateLegalMoves())
}

func (e *Engine) expandRootNode(pos *minixiangqi.Position) ([]*mctsNode, error) {
	legal := pos.GenerateLegalMoves()
	legal = e.FilterVCFMoves(pos, legal)
	return e.expandMoves(pos, legal)
}

func (e *Engine) expandMoves(pos *minixiangqi.Position, legal []minixiangqi.Move) ([]*mctsNode, error) {
	if len(legal) == 0 {
		return nil, nil
	}
	if e.nn == nil {
		children := make([]*mctsNode, 0, len(legal))
		prior := 1.0 / float64(len(legal))
		for _, mv := range legal {
			children = append(children, &mctsNode{move: mv, prior: prior})
		}
		return children, nil
	}

	stage0, err := e.nn.EvaluateWithStage(pos, 0, -1)
	if err != nil {
		return nil, err
	}
	stage1Cache := make(map[int]*NNResult)
	total := 0.0
	children := make([]*mctsNode, 0, len(legal))
	for _, mv := range legal {
		res1 := stage1Cache[mv.From]
		if res1 == nil {
			res1, err = e.nn.EvaluateWithStage(pos, 1, mv.From)
			if err != nil {
				return nil, err
			}
			stage1Cache[mv.From] = res1
		}
		prior := float64(max32(stage0.Policy[mv.From], 0)) * float64(max32(res1.Policy[mv.To], 0))
		children = append(children, &mctsNode{move: mv, prior: prior})
		total += prior
	}
	if total <= 0 {
		total = float64(len(children))
		for i := range children {
			children[i].prior = 1
		}
	}
	for i := range children {
		children[i].prior /= total
	}
	return children, nil
}

func max32(v, lower float32) float32 {
	if v < lower {
		return lower
	}
	return v
}

func sqrt(v float64) float64 {
	if v <= 0 {
		return 0
	}
	z := v
	for i := 0; i < 8; i++ {
		z -= (z*z - v) / (2 * z)
	}
	return z
}
