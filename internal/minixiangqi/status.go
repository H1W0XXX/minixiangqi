package minixiangqi

type Status string

const (
	StatusOngoing  Status = "ongoing"
	StatusRedWin   Status = "red_win"
	StatusBlackWin Status = "black_win"
	StatusDraw     Status = "draw"
)

func statusForWinner(winner Side) Status {
	if winner == Red {
		return StatusRedWin
	}
	return StatusBlackWin
}

func (p *Position) CheckWinnerAfterLastMove() Status {
	if len(p.HalfMoveHistory) == 0 {
		return StatusOngoing
	}
	lastMover := p.SideToMove.Opponent()
	lastLoc := p.HalfMoveHistory[len(p.HalfMoveHistory)-1].Loc

	if !p.KingExists(p.SideToMove) {
		return statusForWinner(lastMover)
	}

	switch p.Rules.LoopRule {
	case LoopRuleSeventhThree:
		if p.IsThreefoldRepetitionDraw() {
			return StatusDraw
		}
	case LoopRuleTwoOne:
		maxLen := 3
		noCaptureTurn := p.MoveNumSinceCapture / 2
		moveHist := p.Get73RuleHistory(lastMover, 0, -1, minInt(noCaptureTurn, maxLen))
		if len(moveHist) > 0 {
			count := 0
			for _, loc := range moveHist {
				if loc == lastLoc {
					count++
				}
			}
			if count >= 2 {
				return statusForWinner(lastMover.Opponent())
			}
		}
	case LoopRuleFiveTwo:
		maxLen := 6
		noCaptureTurn := p.MoveNumSinceCapture / 2
		moveHist := p.Get73RuleHistory(lastMover, 0, -1, minInt(noCaptureTurn, maxLen))
		if len(moveHist) > 0 {
			count := 0
			for _, loc := range moveHist {
				if loc == lastLoc {
					count++
				}
			}
			if count >= 3 {
				return statusForWinner(lastMover.Opponent())
			}
		}
	case LoopRuleRepeatEnd:
		if winner, ok := p.CheckRepeatEndWinner(); ok {
			if winner == drawWinner {
				return StatusDraw
			}
			return statusForWinner(Side(winner))
		}
	}

	if p.Rules.MaxMoves != 0 && p.Rules.MaxMoves <= p.MoveNum {
		return StatusDraw
	}
	if p.Rules.MaxMovesNoCapture != 0 && p.Rules.MaxMovesNoCapture <= p.MoveNumSinceCapture {
		return StatusDraw
	}
	return StatusOngoing
}

func EvaluateStatus(pos *Position) Status {
	if !pos.KingExists(Red) {
		return StatusBlackWin
	}
	if !pos.KingExists(Black) {
		return StatusRedWin
	}
	if status := pos.CheckWinnerAfterLastMove(); status != StatusOngoing {
		return status
	}
	if len(pos.GenerateLegalMoves()) == 0 {
		return statusForWinner(pos.SideToMove.Opponent())
	}
	return StatusOngoing
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

