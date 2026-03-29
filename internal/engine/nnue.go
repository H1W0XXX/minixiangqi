package engine

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"

	"minixiangqi/internal/minixiangqi"
)

const (
	nnueVersion                = 0x7AF32F20
	nnueFeatureHash            = 0x5F234CB8
	nnueFeatureTransformerHash = nnueFeatureHash ^ (nnueL1 * 2)
	nnueL1                     = 512
	nnueL2                     = 16
	nnueL3                     = 32
	nnueKingSquares            = 9
	nnuePiecePlanes            = 9
	nnuePlanesPerKing          = minixiangqi.NumSquares * nnuePiecePlanes
	nnueFeatureCount           = nnuePlanesPerKing * nnueKingSquares
	nnueLayerBuckets           = 8
	nnuePSQTBuckets            = 8
	nnueInputSize              = nnueL1 * 2
	nnueActivationScale        = 127.0
	nnueHiddenScale            = 8128.0
	nnueOutputBiasScale        = 9600.0
	nnueOutputWeightScale      = nnueOutputBiasScale / nnueActivationScale
	nnueScoreScale             = 600.0
	nnueMaxPieces              = 24
)

type nnueLayerStack struct {
	l1Bias    []float32
	l1Weight  []float32
	l2Bias    []float32
	l2Weight  []float32
	outBias   float32
	outWeight []float32
}

type nnueAccumulator struct {
	white     [nnueL1]int32
	black     [nnueL1]int32
	whitePSQT [nnuePSQTBuckets]int32
	blackPSQT [nnuePSQTBuckets]int32
}

type NNUEEvaluator struct {
	ModelPath   string
	SourceDir   string
	Description string
	ftBias      []int16
	ftWeight    []int16
	ftPSQT      []int32
	layerStacks [nnueLayerBuckets]nnueLayerStack
}

func NewNNUEEvaluator(modelPath, sourceDir string) (*NNUEEvaluator, error) {
	absModel, err := filepath.Abs(modelPath)
	if err != nil {
		return nil, fmt.Errorf("resolve nnue model path: %w", err)
	}
	file, err := os.Open(absModel)
	if err != nil {
		return nil, fmt.Errorf("open nnue: %w", err)
	}
	defer file.Close()

	info := &NNUEEvaluator{
		ModelPath: absModel,
		ftBias:    make([]int16, nnueL1),
		ftWeight:  make([]int16, nnueFeatureCount*nnueL1),
		ftPSQT:    make([]int32, nnueFeatureCount*nnuePSQTBuckets),
	}
	if sourceDir != "" {
		if absSource, err := filepath.Abs(sourceDir); err == nil {
			info.SourceDir = absSource
		} else {
			info.SourceDir = sourceDir
		}
	}

	r := bufio.NewReader(file)
	if err := readExpectedUint32(r, nnueVersion, "nnue version"); err != nil {
		return nil, err
	}
	if err := readExpectedUint32(r, nnueNetworkHash(), "nnue network hash"); err != nil {
		return nil, err
	}
	descLen, err := readUint32(r)
	if err != nil {
		return nil, err
	}
	desc := make([]byte, descLen)
	if _, err := io.ReadFull(r, desc); err != nil {
		return nil, fmt.Errorf("read nnue description: %w", err)
	}
	info.Description = string(desc)
	if err := readExpectedUint32(r, nnueFeatureTransformerHash, "feature transformer hash"); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, info.ftBias); err != nil {
		return nil, fmt.Errorf("read ft bias: %w", err)
	}
	if err := binary.Read(r, binary.LittleEndian, info.ftWeight); err != nil {
		return nil, fmt.Errorf("read ft weight: %w", err)
	}
	if err := binary.Read(r, binary.LittleEndian, info.ftPSQT); err != nil {
		return nil, fmt.Errorf("read ft psqt: %w", err)
	}

	for i := 0; i < nnueLayerBuckets; i++ {
		if err := readExpectedUint32(r, nnueFCHash(), "fc layer hash"); err != nil {
			return nil, err
		}
		stack, err := readNNUELayerStack(r)
		if err != nil {
			return nil, err
		}
		info.layerStacks[i] = stack
	}

	log.Printf("NNUE ready: %s", absModel)
	return info, nil
}

func (n *NNUEEvaluator) Evaluate(pos *minixiangqi.Position) (int, error) {
	if n == nil {
		return 0, fmt.Errorf("nnue not initialized")
	}
	if pos == nil {
		return 0, fmt.Errorf("position is nil")
	}
	acc, err := n.NewAccumulator(pos)
	if err != nil {
		return 0, err
	}
	return n.EvaluateWithAccumulator(pos, &acc)
}

func (n *NNUEEvaluator) NewAccumulator(pos *minixiangqi.Position) (nnueAccumulator, error) {
	var acc nnueAccumulator
	for i := 0; i < nnueL1; i++ {
		acc.white[i] = int32(n.ftBias[i])
		acc.black[i] = int32(n.ftBias[i])
	}
	if err := n.accumulatePerspective(pos, minixiangqi.Red, &acc.white, &acc.whitePSQT); err != nil {
		return nnueAccumulator{}, err
	}
	if err := n.accumulatePerspective(pos, minixiangqi.Black, &acc.black, &acc.blackPSQT); err != nil {
		return nnueAccumulator{}, err
	}
	return acc, nil
}

func (n *NNUEEvaluator) EvaluateWithAccumulator(pos *minixiangqi.Position, acc *nnueAccumulator) (int, error) {
	if n == nil {
		return 0, fmt.Errorf("nnue not initialized")
	}
	if pos == nil {
		return 0, fmt.Errorf("position is nil")
	}
	if acc == nil {
		fresh, err := n.NewAccumulator(pos)
		if err != nil {
			return 0, err
		}
		acc = &fresh
	}
	var input [nnueInputSize]float32
	if pos.SideToMove == minixiangqi.Red {
		for i := 0; i < nnueL1; i++ {
			input[i] = clippedFeatureValue(acc.white[i])
			input[nnueL1+i] = clippedFeatureValue(acc.black[i])
		}
	} else {
		for i := 0; i < nnueL1; i++ {
			input[i] = clippedFeatureValue(acc.black[i])
			input[nnueL1+i] = clippedFeatureValue(acc.white[i])
		}
	}

	bucket := nnueBucketIndex(pos.TotalPieces())
	stack := n.layerStacks[bucket]

	var h1 [nnueL2]float32
	for out := 0; out < nnueL2; out++ {
		sum := stack.l1Bias[out]
		base := out * nnueInputSize
		for i := 0; i < nnueInputSize; i++ {
			sum += stack.l1Weight[base+i] * input[i]
		}
		h1[out] = clampUnit(sum)
	}

	var h2 [nnueL3]float32
	for out := 0; out < nnueL3; out++ {
		sum := stack.l2Bias[out]
		base := out * nnueL2
		for i := 0; i < nnueL2; i++ {
			sum += stack.l2Weight[base+i] * h1[i]
		}
		h2[out] = clampUnit(sum)
	}

	psqtTerm := float32(acc.whitePSQT[bucket]-acc.blackPSQT[bucket]) / float32(nnueOutputBiasScale)
	if pos.SideToMove == minixiangqi.Red {
		psqtTerm *= 0.5
	} else {
		psqtTerm *= -0.5
	}

	out := stack.outBias + psqtTerm
	for i := 0; i < nnueL3; i++ {
		out += stack.outWeight[i] * h2[i]
	}

	score := int(math.Round(float64(out * nnueScoreScale)))
	if pos.SideToMove == minixiangqi.Black {
		score = -score
	}
	return score, nil
}

func (n *NNUEEvaluator) DeriveAccumulator(parent *minixiangqi.Position, child *minixiangqi.Position, mv minixiangqi.Move, parentAcc *nnueAccumulator) (nnueAccumulator, error) {
	if n == nil {
		return nnueAccumulator{}, fmt.Errorf("nnue not initialized")
	}
	if parentAcc == nil {
		return n.NewAccumulator(child)
	}
	if parent == nil || child == nil {
		return nnueAccumulator{}, fmt.Errorf("position is nil")
	}

	moving := parent.Board[mv.From]
	captured := parent.Board[mv.To]
	if moving == minixiangqi.Empty {
		return nnueAccumulator{}, fmt.Errorf("no moving piece on square %d", mv.From)
	}

	next := *parentAcc
	if err := n.updatePerspectiveIncremental(parent, child, mv, moving, captured, minixiangqi.Red, &next.white, &next.whitePSQT); err != nil {
		return nnueAccumulator{}, err
	}
	if err := n.updatePerspectiveIncremental(parent, child, mv, moving, captured, minixiangqi.Black, &next.black, &next.blackPSQT); err != nil {
		return nnueAccumulator{}, err
	}
	return next, nil
}

func (n *NNUEEvaluator) accumulatePerspective(pos *minixiangqi.Position, pov minixiangqi.Side, acc *[nnueL1]int32, psqt *[nnuePSQTBuckets]int32) error {
	kingSq := pos.FindKing(pov)
	if kingSq < 0 {
		return fmt.Errorf("missing %v king", pov)
	}
	kingIndex, err := nnueKingIndex(pov, kingSq)
	if err != nil {
		return err
	}
	for sq, piece := range pos.Board {
		if piece == minixiangqi.Empty {
			continue
		}
		feature := nnueFeatureIndex(pov, kingIndex, sq, piece)
		if feature < 0 || feature >= nnueFeatureCount {
			return fmt.Errorf("nnue feature out of range: %d", feature)
		}
		wBase := feature * nnueL1
		for i := 0; i < nnueL1; i++ {
			acc[i] += int32(n.ftWeight[wBase+i])
		}
		pBase := feature * nnuePSQTBuckets
		for i := 0; i < nnuePSQTBuckets; i++ {
			psqt[i] += n.ftPSQT[pBase+i]
		}
	}
	return nil
}

func (n *NNUEEvaluator) recomputePerspective(pos *minixiangqi.Position, pov minixiangqi.Side, acc *[nnueL1]int32, psqt *[nnuePSQTBuckets]int32) error {
	for i := 0; i < nnueL1; i++ {
		acc[i] = int32(n.ftBias[i])
	}
	for i := 0; i < nnuePSQTBuckets; i++ {
		psqt[i] = 0
	}
	return n.accumulatePerspective(pos, pov, acc, psqt)
}

func (n *NNUEEvaluator) updatePerspectiveIncremental(parent *minixiangqi.Position, child *minixiangqi.Position, mv minixiangqi.Move, moving, captured minixiangqi.Piece, pov minixiangqi.Side, acc *[nnueL1]int32, psqt *[nnuePSQTBuckets]int32) error {
	if moving.Type() == minixiangqi.PieceKing && moving.Side() == pov {
		return n.recomputePerspective(child, pov, acc, psqt)
	}
	kingSq := parent.FindKing(pov)
	kingIndex, err := nnueKingIndex(pov, kingSq)
	if err != nil {
		return err
	}
	if err := n.addFeatureDelta(pov, kingIndex, mv.From, moving, -1, acc, psqt); err != nil {
		return err
	}
	if captured != minixiangqi.Empty {
		if err := n.addFeatureDelta(pov, kingIndex, mv.To, captured, -1, acc, psqt); err != nil {
			return err
		}
	}
	if err := n.addFeatureDelta(pov, kingIndex, mv.To, child.Board[mv.To], 1, acc, psqt); err != nil {
		return err
	}
	return nil
}

func (n *NNUEEvaluator) addFeatureDelta(pov minixiangqi.Side, kingIndex int, sq int, piece minixiangqi.Piece, sign int32, acc *[nnueL1]int32, psqt *[nnuePSQTBuckets]int32) error {
	if piece == minixiangqi.Empty {
		return nil
	}
	feature := nnueFeatureIndex(pov, kingIndex, sq, piece)
	if feature < 0 || feature >= nnueFeatureCount {
		return fmt.Errorf("nnue feature out of range: %d", feature)
	}
	wBase := feature * nnueL1
	for i := 0; i < nnueL1; i++ {
		acc[i] += sign * int32(n.ftWeight[wBase+i])
	}
	pBase := feature * nnuePSQTBuckets
	for i := 0; i < nnuePSQTBuckets; i++ {
		psqt[i] += sign * n.ftPSQT[pBase+i]
	}
	return nil
}

func clippedFeatureValue(raw int32) float32 {
	if raw <= 0 {
		return 0
	}
	if raw >= int32(nnueActivationScale) {
		return 1
	}
	return float32(raw) / float32(nnueActivationScale)
}

func clampUnit(v float32) float32 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 1
	}
	return v
}

func nnueBucketIndex(pieceCount int) int {
	if pieceCount <= 0 {
		return 0
	}
	bucket := (pieceCount - 1) * nnueLayerBuckets / nnueMaxPieces
	if bucket < 0 {
		return 0
	}
	if bucket >= nnueLayerBuckets {
		return nnueLayerBuckets - 1
	}
	return bucket
}

func nnueKingIndex(pov minixiangqi.Side, sq int) (int, error) {
	oriented := nnueOrientSquare(pov, sq)
	if oriented < 0 || oriented >= minixiangqi.NumSquares {
		return 0, fmt.Errorf("invalid king square: %d", sq)
	}
	row, _ := minixiangqi.RowCol(oriented)
	mapped := (oriented - 4*row - 2) % nnueKingSquares
	if mapped < 0 {
		mapped += nnueKingSquares
	}
	if mapped < 0 || mapped >= nnueKingSquares {
		return 0, fmt.Errorf("invalid mapped king square: %d", oriented)
	}
	return mapped, nil
}

func nnueFeatureIndex(pov minixiangqi.Side, kingIndex int, sq int, piece minixiangqi.Piece) int {
	orientedSq := nnueOrientSquare(pov, sq)
	pieceIndex := nnuePieceIndex(pov, piece)
	return orientedSq + pieceIndex*minixiangqi.NumSquares + kingIndex*nnuePlanesPerKing
}

func nnuePieceIndex(pov minixiangqi.Side, piece minixiangqi.Piece) int {
	idx := nnuePieceBaseIndex(piece.Type())
	if piece.Side() != pov {
		idx++
	}
	if idx == nnuePiecePlanes {
		idx--
	}
	return idx
}

func nnuePieceBaseIndex(pt minixiangqi.PieceType) int {
	// The trainer still uses Fairy-Stockfish's generic piece slots and the
	// local mini-xiangqi variant remaps our 4 non-king pieces onto them via
	// variant.py piece values:
	// slot 0 -> rook, slot 1 -> cannon, slot 2 -> pawn, slot 3 -> horse.
	switch pt {
	case minixiangqi.PieceRook:
		return 0
	case minixiangqi.PieceCannon:
		return 2
	case minixiangqi.PiecePawn:
		return 4
	case minixiangqi.PieceHorse:
		return 6
	case minixiangqi.PieceKing:
		return 8
	default:
		return 0
	}
}

func nnueOrientSquare(pov minixiangqi.Side, sq int) int {
	if pov == minixiangqi.Red || sq < 0 || sq >= minixiangqi.NumSquares {
		return sq
	}
	row, col := minixiangqi.RowCol(sq)
	return minixiangqi.Square(minixiangqi.Rows-1-row, col)
}

func readNNUELayerStack(r io.Reader) (nnueLayerStack, error) {
	l1Bias, l1Weight, err := readNNUEFCLayer(r, nnueL2, nnueInputSize, false)
	if err != nil {
		return nnueLayerStack{}, err
	}
	l2Bias, l2Weight, err := readNNUEFCLayer(r, nnueL3, nnueL2, false)
	if err != nil {
		return nnueLayerStack{}, err
	}
	outBias, outWeight, err := readNNUEFCLayer(r, 1, nnueL3, true)
	if err != nil {
		return nnueLayerStack{}, err
	}
	return nnueLayerStack{
		l1Bias:    l1Bias,
		l1Weight:  l1Weight,
		l2Bias:    l2Bias,
		l2Weight:  l2Weight,
		outBias:   outBias[0],
		outWeight: outWeight,
	}, nil
}

func readNNUEFCLayer(r io.Reader, outputs, inputs int, isOutput bool) ([]float32, []float32, error) {
	biasScale := float32(nnueHiddenScale)
	weightScale := float32(64.0)
	if isOutput {
		biasScale = float32(nnueOutputBiasScale)
		weightScale = float32(nnueOutputWeightScale)
	}

	biasRaw := make([]int32, outputs)
	if err := binary.Read(r, binary.LittleEndian, biasRaw); err != nil {
		return nil, nil, fmt.Errorf("read fc bias: %w", err)
	}
	bias := make([]float32, outputs)
	for i := range biasRaw {
		bias[i] = float32(biasRaw[i]) / biasScale
	}

	paddedInputs := ((inputs + 31) / 32) * 32
	weightsRaw, err := readInt8Buffer(r, outputs*paddedInputs)
	if err != nil {
		return nil, nil, fmt.Errorf("read fc weight: %w", err)
	}
	weights := make([]float32, outputs*inputs)
	for out := 0; out < outputs; out++ {
		src := out * paddedInputs
		dst := out * inputs
		for in := 0; in < inputs; in++ {
			weights[dst+in] = float32(weightsRaw[src+in]) / weightScale
		}
	}
	return bias, weights, nil
}

func readInt8Buffer(r io.Reader, n int) ([]int8, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	out := make([]int8, n)
	for i := range buf {
		out[i] = int8(buf[i])
	}
	return out, nil
}

func readUint32(r io.Reader) (uint32, error) {
	var v uint32
	if err := binary.Read(r, binary.LittleEndian, &v); err != nil {
		return 0, err
	}
	return v, nil
}

func readExpectedUint32(r io.Reader, expected uint32, label string) error {
	v, err := readUint32(r)
	if err != nil {
		return fmt.Errorf("read %s: %w", label, err)
	}
	if v != expected {
		return fmt.Errorf("%s mismatch: got 0x%08x want 0x%08x", label, v, expected)
	}
	return nil
}

func nnueNetworkHash() uint32 {
	return nnueFCHash() ^ nnueFeatureHash ^ (nnueL1 * 2)
}

func nnueFCHash() uint32 {
	prev := uint32(0xEC42E90D ^ (nnueL1 * 2))
	for _, outFeatures := range []uint32{nnueL2, nnueL3, 1} {
		layerHash := uint32(0xCC03DAE4) + outFeatures
		layerHash ^= prev >> 1
		layerHash ^= prev << 31
		if outFeatures != 1 {
			layerHash += 0x538D24C7
		}
		prev = layerHash
	}
	return prev
}
