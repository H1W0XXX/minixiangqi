package minixiangqi

var (
	zobristPieces      [14][NumSquares]uint64
	zobristBlackToMove uint64
)

func init() {
	seed := uint64(0x9E3779B97F4A7C15)
	for piece := range zobristPieces {
		for sq := 0; sq < NumSquares; sq++ {
			zobristPieces[piece][sq] = nextSplitMix64(&seed)
		}
	}
	zobristBlackToMove = nextSplitMix64(&seed)
}

func nextSplitMix64(seed *uint64) uint64 {
	*seed += 0x9E3779B97F4A7C15
	z := *seed
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

func zobristPieceHash(piece Piece, sq int) uint64 {
	if piece == Empty || sq < 0 || sq >= NumSquares {
		return 0
	}
	return zobristPieces[int(piece)][sq]
}

func zobristSideHash(side Side) uint64 {
	if side == Black {
		return zobristBlackToMove
	}
	return 0
}
