package minixiangqi

const (
	Rows       = 7
	Cols       = 7
	NumSquares = Rows * Cols
)

type Side uint8

const (
	Red Side = iota
	Black
)

func (s Side) Opponent() Side {
	if s == Black {
		return Red
	}
	return Black
}

type PieceType uint8

const (
	PieceNone PieceType = iota
	PiecePawn
	PieceRook
	PieceCannon
	PieceHorse
	PieceKing
)

type Piece uint8

const (
	Empty Piece = 0

	RedPawn   Piece = 1
	RedRook   Piece = 2
	RedCannon Piece = 3
	RedHorse  Piece = 4
	RedKing   Piece = 5

	BlackPawn   Piece = 9
	BlackRook   Piece = 10
	BlackCannon Piece = 11
	BlackHorse  Piece = 12
	BlackKing   Piece = 13
)

func (p Piece) Type() PieceType {
	switch p {
	case RedPawn, BlackPawn:
		return PiecePawn
	case RedRook, BlackRook:
		return PieceRook
	case RedCannon, BlackCannon:
		return PieceCannon
	case RedHorse, BlackHorse:
		return PieceHorse
	case RedKing, BlackKing:
		return PieceKing
	default:
		return PieceNone
	}
}

func (p Piece) Side() Side {
	if p >= BlackPawn {
		return Black
	}
	return Red
}

type Move struct {
	From int
	To   int
}

type HalfMove struct {
	Loc  int
	Side Side
}

type Rules struct {
	ScoringRule        int
	DrawJudgeRule      int
	LoopRule           int
	MaxMoves           int
	MaxMovesNoCapture  int
}

const (
	Scoring0 = iota
	Scoring1
	Scoring2
	Scoring3
)

const (
	DrawJudgeDraw = iota
	DrawJudgeCount
	DrawJudgeWeight
)

const (
	LoopRuleSeventhThree = iota
	LoopRuleNone
	LoopRuleRepeatEnd
	LoopRuleTwoOne
	LoopRuleFiveTwo
)

func DefaultRules() Rules {
	return Rules{
		ScoringRule:       Scoring0,
		DrawJudgeRule:     DrawJudgeDraw,
		LoopRule:          LoopRuleSeventhThree,
		MaxMoves:          0,
		MaxMovesNoCapture: 200,
	}
}

type Position struct {
	Board               [NumSquares]Piece
	SideToMove          Side
	Hash                uint64
	MoveNum             int
	MoveNumSinceCapture int
	Rules               Rules
	HalfMoveHistory     []HalfMove
	PositionHashes      []uint64
}

const InitialFEN = "cpmwmpc/b1bbb1b/7/7/7/B1BBB1B/CPMWMPC w"
