package engine

import (
	"path/filepath"
	"testing"

	"minixiangqi/internal/minixiangqi"
)

func TestNNUEAccumulatorMatchesFullRecompute(t *testing.T) {
	modelPath := filepath.Join("..", "..", "minixiangqi.nnue")
	nnue, err := NewNNUEEvaluator(modelPath, "")
	if err != nil {
		t.Skipf("skip nnue test: %v", err)
	}

	pos := minixiangqi.NewInitialPosition()
	rootAcc, err := nnue.NewAccumulator(pos)
	if err != nil {
		t.Fatalf("new accumulator: %v", err)
	}
	rootScore, err := nnue.EvaluateWithAccumulator(pos, &rootAcc)
	if err != nil {
		t.Fatalf("eval root with accumulator: %v", err)
	}
	fullRootScore, err := nnue.Evaluate(pos)
	if err != nil {
		t.Fatalf("eval root full: %v", err)
	}
	if rootScore != fullRootScore {
		t.Fatalf("root mismatch: acc=%d full=%d", rootScore, fullRootScore)
	}

	moves := pos.GenerateLegalMoves()
	limit := 8
	if len(moves) < limit {
		limit = len(moves)
	}
	for i := 0; i < limit; i++ {
		mv := moves[i]
		child, ok := pos.ApplyMove(mv)
		if !ok {
			t.Fatalf("apply move failed: %+v", mv)
		}
		childAcc, err := nnue.DeriveAccumulator(pos, child, mv, &rootAcc)
		if err != nil {
			t.Fatalf("derive accumulator for %+v: %v", mv, err)
		}
		accScore, err := nnue.EvaluateWithAccumulator(child, &childAcc)
		if err != nil {
			t.Fatalf("eval child with accumulator %+v: %v", mv, err)
		}
		fullScore, err := nnue.Evaluate(child)
		if err != nil {
			t.Fatalf("eval child full %+v: %v", mv, err)
		}
		if accScore != fullScore {
			t.Fatalf("move %+v mismatch: acc=%d full=%d", mv, accScore, fullScore)
		}
	}
}

func TestNNUEPieceIndexMapping(t *testing.T) {
	tests := []struct {
		name  string
		piece minixiangqi.Piece
		want  int
	}{
		{name: "red rook", piece: minixiangqi.RedRook, want: 0},
		{name: "black rook", piece: minixiangqi.BlackRook, want: 1},
		{name: "red cannon", piece: minixiangqi.RedCannon, want: 2},
		{name: "black cannon", piece: minixiangqi.BlackCannon, want: 3},
		{name: "red pawn", piece: minixiangqi.RedPawn, want: 4},
		{name: "black pawn", piece: minixiangqi.BlackPawn, want: 5},
		{name: "red horse", piece: minixiangqi.RedHorse, want: 6},
		{name: "black horse", piece: minixiangqi.BlackHorse, want: 7},
		{name: "red king", piece: minixiangqi.RedKing, want: 8},
		{name: "black king", piece: minixiangqi.BlackKing, want: 8},
	}

	for _, tc := range tests {
		if got := nnuePieceIndex(minixiangqi.Red, tc.piece); got != tc.want {
			t.Fatalf("%s: got %d want %d", tc.name, got, tc.want)
		}
	}
}
