package minixiangqi

import "testing"

func TestInitialPositionHashRecorded(t *testing.T) {
	pos := NewInitialPosition()
	if len(pos.PositionHashes) != 1 {
		t.Fatalf("expected initial hash history length 1, got %d", len(pos.PositionHashes))
	}
	if pos.PositionHashes[0] != pos.EnsureHash() {
		t.Fatalf("initial hash mismatch: hist=%d current=%d", pos.PositionHashes[0], pos.EnsureHash())
	}
}

func TestGenerateLegalMovesFiltersFourthRepetition(t *testing.T) {
	pos, err := DecodePosition("c1w4/7/7/7/7/7/4W1C w")
	if err != nil {
		t.Fatalf("decode position: %v", err)
	}

	sequence := []Move{
		{From: 48, To: 47},
		{From: 0, To: 1},
		{From: 47, To: 48},
		{From: 1, To: 0},
		{From: 48, To: 47},
		{From: 0, To: 1},
		{From: 47, To: 48},
		{From: 1, To: 0},
		{From: 48, To: 47},
		{From: 0, To: 1},
		{From: 47, To: 48},
	}

	for i, mv := range sequence {
		next, ok := pos.ApplyMove(mv)
		if !ok {
			t.Fatalf("apply move %d failed: %+v", i, mv)
		}
		pos = next
	}

	if pos.SideToMove != Black {
		t.Fatalf("expected black to move, got %v", pos.SideToMove)
	}

	for _, mv := range pos.GenerateLegalMoves() {
		if mv == (Move{From: 1, To: 0}) {
			t.Fatalf("move recreating the same position for the 4th time was not filtered")
		}
	}
}
