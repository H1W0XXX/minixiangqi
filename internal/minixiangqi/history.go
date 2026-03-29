package minixiangqi

const (
	invalidWinner = -2
	drawWinner    = -1
)

type ResultsBeforeNN struct {
	Inited   bool
	Winner   int
	MyOnlyLoc int
}

func (p *Position) rule73Length() int {
	switch p.Rules.LoopRule {
	case LoopRuleSeventhThree:
		return 7
	case LoopRuleNone:
		return 0
	case LoopRuleRepeatEnd:
		return 7
	case LoopRuleTwoOne:
		return 2
	case LoopRuleFiveTwo:
		return 5
	default:
		return 0
	}
}

func (p *Position) stageHistory(stage int, chosenSquare int) []HalfMove {
	if stage != 1 || chosenSquare < 0 || chosenSquare >= NumSquares {
		return p.HalfMoveHistory
	}
	history := make([]HalfMove, len(p.HalfMoveHistory)+1)
	copy(history, p.HalfMoveHistory)
	history[len(p.HalfMoveHistory)] = HalfMove{Loc: chosenSquare, Side: p.SideToMove}
	return history
}

func (p *Position) Get73RuleHistory(side Side, stage int, chosenSquare int, maxLen int) []int {
	if maxLen <= 0 {
		return nil
	}
	history := p.stageHistory(stage, chosenSquare)
	t := len(history) - 1
	switch {
	case side == p.SideToMove && stage == 1:
		t = len(history) - 4
		if t < 0 {
			return nil
		}
		if history[t].Side != side || history[t].Loc != chosenSquare {
			return nil
		}
	case side != p.SideToMove && stage == 0:
		t = len(history) - 1
	case side != p.SideToMove && stage == 1:
		t = len(history) - 2
	case side == p.SideToMove && stage == 0:
		t = len(history) - 3
	}
	if t < 0 {
		return nil
	}

	out := make([]int, 0, maxLen)
	nowLoc := history[t].Loc
	for len(out) < maxLen && t >= 0 {
		if !OnSquare(nowLoc) {
			return out
		}
		if t-1 < 0 || history[t].Side != side || history[t-1].Side != side {
			return out
		}
		if nowLoc != history[t].Loc {
			break
		}
		out = append(out, history[t].Loc)
		nowLoc = history[t-1].Loc
		t -= 4
	}
	return out
}

func (p *Position) IsThreefoldRepetitionDraw() bool {
	current := p.EnsureHash()
	count := 0
	for _, h := range p.PositionHashes {
		if h == current {
			count++
		}
	}
	if len(p.PositionHashes) == 0 || p.PositionHashes[len(p.PositionHashes)-1] != current {
		count++
	}
	return count >= 3
}

func (p *Position) CheckRepeatEndWinner() (int, bool) {
	if len(p.PositionHashes) == 0 {
		return 0, false
	}
	lastHash := p.PositionHashes[len(p.PositionHashes)-1]
	repeatLength := 0
	for turn := len(p.PositionHashes) - 3; turn >= 0; turn -= 2 {
		if len(p.PositionHashes)-turn-1 > 18 {
			break
		}
		if p.PositionHashes[turn] == lastHash {
			repeatLength = len(p.PositionHashes) - turn - 1
			break
		}
	}
	if repeatLength == 0 {
		return 0, false
	}

	repeatTurn := repeatLength / 2
	for i := 0; i < repeatTurn*4; i++ {
		idx := len(p.HalfMoveHistory) - 1 - i
		if idx < 0 || !OnSquare(p.HalfMoveHistory[idx].Loc) {
			return 0, false
		}
	}

	moveHistIdx0 := len(p.HalfMoveHistory) - 1
	moveHistIdx1 := len(p.HalfMoveHistory) - 1 - 4*repeatTurn
	if moveHistIdx0 < 0 || moveHistIdx1 < 0 {
		return 0, false
	}
	if p.HalfMoveHistory[moveHistIdx0].Loc == p.HalfMoveHistory[moveHistIdx1].Loc {
		return int(p.SideToMove), true
	}
	return int(p.SideToMove.Opponent()), true
}

func (p *Position) GetResultsBeforeNN(stage int, chosenSquare int) ResultsBeforeNN {
	res := ResultsBeforeNN{
		Inited:    true,
		Winner:    invalidWinner,
		MyOnlyLoc: -1,
	}
	for target := 0; target < NumSquares; target++ {
		if p.MovePriority(stage, chosenSquare, target) == MovePrioritySuddenWin {
			res.Winner = int(p.SideToMove)
			res.MyOnlyLoc = target
			return res
		}
	}
	return res
}

