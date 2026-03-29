package minixiangqi

import (
	"fmt"
	"strconv"
	"strings"
)

func NewInitialPosition() *Position {
	pos, err := DecodePosition(InitialFEN)
	if err != nil {
		panic(err)
	}
	return pos
}

func DecodePosition(fen string) (*Position, error) {
	parts := strings.Fields(strings.TrimSpace(fen))
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid fen: %q", fen)
	}
	rows := strings.Split(parts[0], "/")
	if len(rows) != Rows {
		return nil, fmt.Errorf("invalid board rows")
	}
	pos := &Position{}
	for r, row := range rows {
		c := 0
		for _, ch := range row {
			if ch >= '1' && ch <= '9' {
				c += int(ch - '0')
				continue
			}
			if c >= Cols {
				return nil, fmt.Errorf("row %d too wide", r)
			}
			piece, ok := pieceFromFEN(byte(ch))
			if !ok {
				return nil, fmt.Errorf("invalid piece %q", string(ch))
			}
			pos.Board[r*Cols+c] = piece
			c++
		}
		if c != Cols {
			return nil, fmt.Errorf("row %d width mismatch", r)
		}
	}
	switch parts[1] {
	case "w":
		pos.SideToMove = Red
	case "b":
		pos.SideToMove = Black
	default:
		return nil, fmt.Errorf("invalid side")
	}
	pos.Rules = DefaultRules()
	pos.Hash = pos.CalculateHash()
	return pos, nil
}

func (p *Position) Encode() string {
	var b strings.Builder
	for r := 0; r < Rows; r++ {
		if r > 0 {
			b.WriteByte('/')
		}
		empty := 0
		for c := 0; c < Cols; c++ {
			piece := p.Board[r*Cols+c]
			if piece == Empty {
				empty++
				continue
			}
			if empty > 0 {
				b.WriteString(strconv.Itoa(empty))
				empty = 0
			}
			b.WriteByte(pieceToFEN(piece))
		}
		if empty > 0 {
			b.WriteString(strconv.Itoa(empty))
		}
	}
	if p.SideToMove == Black {
		b.WriteString(" b")
	} else {
		b.WriteString(" w")
	}
	return b.String()
}

func (p *Position) Clone() *Position {
	cp := *p
	if p.HalfMoveHistory != nil {
		cp.HalfMoveHistory = append([]HalfMove(nil), p.HalfMoveHistory...)
	}
	if p.PositionHashes != nil {
		cp.PositionHashes = append([]uint64(nil), p.PositionHashes...)
	}
	return &cp
}

func (p *Position) CalculateHash() uint64 {
	var h uint64
	for sq, piece := range p.Board {
		if piece == Empty {
			continue
		}
		h ^= zobristPieceHash(piece, sq)
	}
	h ^= zobristSideHash(p.SideToMove)
	return h
}

func (p *Position) EnsureHash() uint64 {
	if p.Hash == 0 {
		p.Hash = p.CalculateHash()
	}
	return p.Hash
}

func pieceFromFEN(ch byte) (Piece, bool) {
	switch ch {
	case 'B':
		return RedPawn, true
	case 'C':
		return RedRook, true
	case 'P':
		return RedCannon, true
	case 'M':
		return RedHorse, true
	case 'W':
		return RedKing, true
	case 'b':
		return BlackPawn, true
	case 'c':
		return BlackRook, true
	case 'p':
		return BlackCannon, true
	case 'm':
		return BlackHorse, true
	case 'w':
		return BlackKing, true
	default:
		return Empty, false
	}
}

func pieceToFEN(piece Piece) byte {
	switch piece {
	case RedPawn:
		return 'B'
	case RedRook:
		return 'C'
	case RedCannon:
		return 'P'
	case RedHorse:
		return 'M'
	case RedKing:
		return 'W'
	case BlackPawn:
		return 'b'
	case BlackRook:
		return 'c'
	case BlackCannon:
		return 'p'
	case BlackHorse:
		return 'm'
	case BlackKing:
		return 'w'
	default:
		return '1'
	}
}
