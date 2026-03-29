package minixiangqi

type MovePriority int8

const (
	MovePriorityIllegal    MovePriority = -1
	MovePrioritySuddenWin  MovePriority = 1
	MovePriorityOnlyNonLose MovePriority = 2
	MovePriorityWinning    MovePriority = 3
	MovePriorityNormal     MovePriority = 126
)

func OnSquare(sq int) bool {
	return sq >= 0 && sq < NumSquares
}

func (p *Position) IsStageLegal(stage int, chosenSquare int, target int) bool {
	if !OnSquare(target) {
		return false
	}
	if stage == 0 {
		piece := p.Board[target]
		return piece != Empty && piece.Side() == p.SideToMove
	}
	if stage != 1 || !OnSquare(chosenSquare) {
		return false
	}
	for _, mv := range p.GenerateLegalMoves() {
		if mv.From == chosenSquare && mv.To == target {
			return true
		}
	}
	return false
}

func (p *Position) MovePriority(stage int, chosenSquare int, target int) MovePriority {
	if !p.IsStageLegal(stage, chosenSquare, target) {
		return MovePriorityIllegal
	}
	if stage == 1 {
		piece := p.Board[target]
		if piece != Empty && piece.Type() == PieceKing && piece.Side() != p.SideToMove {
			return MovePrioritySuddenWin
		}
	}
	return MovePriorityNormal
}

