package minixiangqi

func RowCol(sq int) (int, int) {
	return sq / Cols, sq % Cols
}

func Square(row, col int) int {
	return row*Cols + col
}

func OnBoard(row, col int) bool {
	return row >= 0 && row < Rows && col >= 0 && col < Cols
}

func InPalace(side Side, row, col int) bool {
	if col < 2 || col > 4 {
		return false
	}
	if side == Black {
		return row >= 0 && row <= 2
	}
	return row >= 4 && row <= 6
}

func ForwardDelta(side Side) int {
	if side == Red {
		return -1
	}
	return 1
}

func (p *Position) TotalPieces() int {
	count := 0
	for _, piece := range p.Board {
		if piece != Empty {
			count++
		}
	}
	return count
}

func (p *Position) KingExists(side Side) bool {
	return p.FindKing(side) >= 0
}

func (p *Position) FindKing(side Side) int {
	target := RedKing
	if side == Black {
		target = BlackKing
	}
	for i, piece := range p.Board {
		if piece == target {
			return i
		}
	}
	return -1
}

func (p *Position) GenerateLegalMoves() []Move {
	if !p.KingExists(p.SideToMove) {
		return nil
	}
	pseudo := p.generatePseudoMovesFor(p.SideToMove)
	out := make([]Move, 0, len(pseudo))
	for _, mv := range pseudo {
		next := p.applyMoveUnchecked(mv)
		if next == nil {
			continue
		}
		if next.IsInCheck(p.SideToMove) {
			continue
		}
		if next.KingsFaceToFace() {
			continue
		}
		out = append(out, mv)
	}
	return out
}

func (p *Position) ApplyMove(m Move) (*Position, bool) {
	for _, legal := range p.GenerateLegalMoves() {
		if legal == m {
			return p.applyMoveUnchecked(m), true
		}
	}
	return nil, false
}

func (p *Position) applyMoveUnchecked(m Move) *Position {
	if m.From < 0 || m.From >= NumSquares || m.To < 0 || m.To >= NumSquares {
		return nil
	}
	moving := p.Board[m.From]
	if moving == Empty || moving.Side() != p.SideToMove {
		return nil
	}
	target := p.Board[m.To]
	if target != Empty && target.Side() == p.SideToMove {
		return nil
	}
	next := p.Clone()
	next.Board[m.To] = moving
	next.Board[m.From] = Empty
	next.SideToMove = p.SideToMove.Opponent()
	next.MoveNum = p.MoveNum + 1
	if target != Empty {
		next.MoveNumSinceCapture = 0
	} else {
		next.MoveNumSinceCapture = p.MoveNumSinceCapture + 1
	}
	next.HalfMoveHistory = append(next.HalfMoveHistory,
		HalfMove{Loc: m.From, Side: p.SideToMove},
		HalfMove{Loc: m.To, Side: p.SideToMove},
	)
	next.Hash = p.EnsureHash()
	next.Hash ^= zobristPieceHash(moving, m.From)
	if target != Empty {
		next.Hash ^= zobristPieceHash(target, m.To)
	}
	next.Hash ^= zobristPieceHash(moving, m.To)
	next.Hash ^= zobristSideHash(Black)
	next.PositionHashes = append(next.PositionHashes, next.Hash)
	return next
}

func (p *Position) GenerateLegalMovesFrom(from int) []Move {
	all := p.GenerateLegalMoves()
	out := make([]Move, 0, len(all))
	for _, mv := range all {
		if mv.From == from {
			out = append(out, mv)
		}
	}
	return out
}

func (p *Position) IsInCheck(side Side) bool {
	kingSq := p.FindKing(side)
	if kingSq < 0 {
		return true
	}
	opp := side.Opponent()
	for _, mv := range p.generatePseudoMovesFor(opp) {
		if mv.To == kingSq {
			return true
		}
	}
	return p.kingsFaceAlongFile()
}

func (p *Position) KingsFaceToFace() bool {
	return p.kingsFaceAlongFile()
}

func (p *Position) kingsFaceAlongFile() bool {
	redKing := p.FindKing(Red)
	blackKing := p.FindKing(Black)
	if redKing < 0 || blackKing < 0 {
		return false
	}
	rr, rc := RowCol(redKing)
	br, bc := RowCol(blackKing)
	if rc != bc {
		return false
	}
	if rr > br {
		rr, br = br, rr
	}
	for row := rr + 1; row < br; row++ {
		if p.Board[Square(row, rc)] != Empty {
			return false
		}
	}
	return true
}

func (p *Position) generatePseudoMovesFor(side Side) []Move {
	out := make([]Move, 0, 64)
	for sq, piece := range p.Board {
		if piece == Empty || piece.Side() != side {
			continue
		}
		switch piece.Type() {
		case PiecePawn:
			p.addPawnMoves(&out, sq, side)
		case PieceRook:
			p.addRookMoves(&out, sq, side)
		case PieceCannon:
			p.addCannonMoves(&out, sq, side)
		case PieceHorse:
			p.addHorseMoves(&out, sq, side)
		case PieceKing:
			p.addKingMoves(&out, sq, side)
		}
	}
	return out
}

func (p *Position) addPawnMoves(out *[]Move, sq int, side Side) {
	row, col := RowCol(sq)
	dirs := [][2]int{
		{ForwardDelta(side), 0},
		{0, -1},
		{0, 1},
	}
	for _, d := range dirs {
		r := row + d[0]
		c := col + d[1]
		if !OnBoard(r, c) {
			continue
		}
		to := Square(r, c)
		if p.Board[to] != Empty && p.Board[to].Side() == side {
			continue
		}
		*out = append(*out, Move{From: sq, To: to})
	}
}

func (p *Position) addRookMoves(out *[]Move, sq int, side Side) {
	row, col := RowCol(sq)
	dirs := [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
	for _, d := range dirs {
		r, c := row+d[0], col+d[1]
		for OnBoard(r, c) {
			to := Square(r, c)
			if p.Board[to] == Empty {
				*out = append(*out, Move{From: sq, To: to})
			} else {
				if p.Board[to].Side() != side {
					*out = append(*out, Move{From: sq, To: to})
				}
				break
			}
			r += d[0]
			c += d[1]
		}
	}
}

func (p *Position) addCannonMoves(out *[]Move, sq int, side Side) {
	row, col := RowCol(sq)
	dirs := [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
	for _, d := range dirs {
		r, c := row+d[0], col+d[1]
		jumped := false
		for OnBoard(r, c) {
			to := Square(r, c)
			piece := p.Board[to]
			if !jumped {
				if piece == Empty {
					*out = append(*out, Move{From: sq, To: to})
				} else {
					jumped = true
				}
			} else if piece != Empty {
				if piece.Side() != side {
					*out = append(*out, Move{From: sq, To: to})
				}
				break
			}
			r += d[0]
			c += d[1]
		}
	}
}

func (p *Position) addHorseMoves(out *[]Move, sq int, side Side) {
	row, col := RowCol(sq)
	steps := [][4]int{
		{-2, -1, -1, 0},
		{-2, 1, -1, 0},
		{2, -1, 1, 0},
		{2, 1, 1, 0},
		{-1, -2, 0, -1},
		{1, -2, 0, -1},
		{-1, 2, 0, 1},
		{1, 2, 0, 1},
	}
	for _, st := range steps {
		lr, lc := row+st[2], col+st[3]
		if !OnBoard(lr, lc) || p.Board[Square(lr, lc)] != Empty {
			continue
		}
		r, c := row+st[0], col+st[1]
		if !OnBoard(r, c) {
			continue
		}
		to := Square(r, c)
		if p.Board[to] != Empty && p.Board[to].Side() == side {
			continue
		}
		*out = append(*out, Move{From: sq, To: to})
	}
}

func (p *Position) addKingMoves(out *[]Move, sq int, side Side) {
	row, col := RowCol(sq)
	dirs := [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
	for _, d := range dirs {
		r, c := row+d[0], col+d[1]
		if !OnBoard(r, c) || !InPalace(side, r, c) {
			continue
		}
		to := Square(r, c)
		if p.Board[to] != Empty && p.Board[to].Side() == side {
			continue
		}
		*out = append(*out, Move{From: sq, To: to})
	}
}
