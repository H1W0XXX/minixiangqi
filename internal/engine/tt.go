package engine

import "minixiangqi/internal/minixiangqi"

const (
	ttExact int8 = iota
	ttUpperBound
	ttLowerBound
)

type ttEntry struct {
	Key   uint64
	Depth int
	Score int
	Flag  int8
	Move  minixiangqi.Move
}

func (e *Engine) storeTT(key uint64, depth int, score int, flag int8, mv minixiangqi.Move) {
	if e == nil {
		return
	}
	if e.tt == nil {
		e.tt = make(map[uint64]ttEntry, 1<<16)
	}
	if len(e.tt) > 1_000_000 {
		e.tt = make(map[uint64]ttEntry, 1<<16)
	}
	old, ok := e.tt[key]
	replace := !ok || depth > old.Depth
	if ok && depth == old.Depth && flag == ttExact && old.Flag != ttExact {
		replace = true
	}
	if replace {
		e.tt[key] = ttEntry{
			Key:   key,
			Depth: depth,
			Score: score,
			Flag:  flag,
			Move:  mv,
		}
	}
}

func ttKeyForPosition(pos *minixiangqi.Position) uint64 {
	if pos == nil {
		return 0
	}
	key := pos.EnsureHash()
	key ^= splitMix64(uint64(pos.MoveNum) + 0x100)
	key ^= splitMix64(uint64(pos.MoveNumSinceCapture) + 0x200)
	key ^= splitMix64(uint64(pos.Rules.ScoringRule) + 0x300)
	key ^= splitMix64(uint64(pos.Rules.DrawJudgeRule) + 0x400)
	key ^= splitMix64(uint64(pos.Rules.LoopRule) + 0x500)
	key ^= splitMix64(uint64(pos.Rules.MaxMoves) + 0x600)
	key ^= splitMix64(uint64(pos.Rules.MaxMovesNoCapture) + 0x700)

	start := len(pos.HalfMoveHistory) - 8
	if start < 0 {
		start = 0
	}
	for i := start; i < len(pos.HalfMoveHistory); i++ {
		hm := pos.HalfMoveHistory[i]
		v := uint64((hm.Loc + 2) & 0xFF)
		if hm.Side == minixiangqi.Black {
			v |= 1 << 15
		}
		key ^= splitMix64(v + uint64(i-start+1)*0x1000)
	}
	return key
}

func splitMix64(x uint64) uint64 {
	x += 0x9E3779B97F4A7C15
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}
