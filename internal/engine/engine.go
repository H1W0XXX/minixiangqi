package engine

import (
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"minixiangqi/internal/minixiangqi"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	NumSpatialFeatures = 48
	NumGlobalFeatures  = 64
	PolicySize         = minixiangqi.NumSquares + 1
	mctsCpuct          = 1.35
)

type SearchConfig struct {
	UseMCTS         bool
	MCTSSimulations int
	MaxDepth        int
	TimeLimit       time.Duration
}

type SearchResult struct {
	BestMove minixiangqi.Move
	Score    int
	WinProb  float32
	Nodes    int64
	TimeUsed time.Duration
	NNFailed bool
}

type BackendKind string

const (
	BackendNone            BackendKind = "none"
	BackendONNX            BackendKind = "onnx"
	BackendNNUE            BackendKind = "nnue"
	BackendNNUEPlaceholder             = BackendNNUE
)

type Engine struct {
	UseNN   bool
	backend BackendKind
	nn      *NNEvaluator
	nnue    *NNUEEvaluator
	tt      map[uint64]ttEntry
	nodes   int64
}

func NewEngine() *Engine {
	return &Engine{
		backend: BackendNone,
		tt:      make(map[uint64]ttEntry, 1<<16),
	}
}

func (e *Engine) CloneForGame() *Engine {
	out := NewEngine()
	out.UseNN = e.UseNN
	out.backend = e.backend
	out.nn = e.nn
	out.nnue = e.nnue
	return out
}

func (e *Engine) InitNN(modelPath, libPath string) error {
	nn, err := NewNNEvaluator(modelPath, libPath)
	if err != nil {
		return err
	}
	e.nn = nn
	e.UseNN = true
	e.backend = BackendONNX
	return nil
}

func (e *Engine) InitNNUE(modelPath, sourceDir string) error {
	nnue, err := NewNNUEEvaluator(modelPath, sourceDir)
	if err != nil {
		return err
	}
	e.nnue = nnue
	e.nn = nil
	e.UseNN = true
	e.backend = BackendNNUE
	return nil
}

func (e *Engine) InitNNUEPlaceholder(modelPath, sourceDir string) error {
	return e.InitNNUE(modelPath, sourceDir)
}

func (e *Engine) BackendName() string {
	if e == nil {
		return string(BackendNone)
	}
	return string(e.backend)
}

func (e *Engine) Search(pos *minixiangqi.Position, cfg SearchConfig) SearchResult {
	if pos == nil {
		return SearchResult{}
	}
	switch e.backend {
	case BackendNNUE:
		cfg.UseMCTS = false
	case BackendONNX:
		cfg.UseMCTS = true
	}
	legal := pos.GenerateLegalMoves()
	for _, mv := range legal {
		target := pos.Board[mv.To]
		if target != minixiangqi.Empty && target.Type() == minixiangqi.PieceKing {
			score := 1_000_000
			winProb := float32(1)
			if pos.SideToMove == minixiangqi.Black {
				score = -1_000_000
				winProb = 0
			}
			return SearchResult{
				BestMove: mv,
				Score:    score,
				WinProb:  winProb,
				Nodes:    1,
			}
		}
	}
	vcfRes := e.VCFSearch(pos, vcfDepthRoot)
	if vcfRes.CanWin {
		score := 900_000
		winProb := float32(1)
		if pos.SideToMove == minixiangqi.Black {
			score = -900_000
			winProb = 0
		}
		return SearchResult{
			BestMove: vcfRes.Move,
			Score:    score,
			WinProb:  winProb,
			Nodes:    100,
		}
	}
	if !cfg.UseMCTS {
		if cfg.MaxDepth <= 0 {
			cfg.MaxDepth = cfg.MCTSSimulations
		}
		if cfg.MaxDepth <= 0 {
			cfg.MaxDepth = 3
		}
		if cfg.TimeLimit <= 0 {
			cfg.TimeLimit = 3 * time.Second
		}
		return e.runAlphaBeta(pos, cfg)
	}
	if cfg.MCTSSimulations <= 0 {
		cfg.MCTSSimulations = 200
	}
	if cfg.TimeLimit <= 0 {
		cfg.TimeLimit = 3 * time.Second
	}
	return e.runMCTS(pos, cfg)
}

type NNResult struct {
	WinProb  float32
	LossProb float32
	DrawProb float32
	Policy   []float32
}

type NNEvaluator struct {
	session     *ort.AdvancedSession
	inputs      []ort.Value
	outputs     []ort.Value
	binInput    []float32
	globalInput []float32
	policy      []float32
	value       []float32
	provider    string
	modelPath   string
	mu          sync.Mutex
	cache       *nnEvalCache
}

func NewNNEvaluator(modelPath, libPath string) (*NNEvaluator, error) {
	absModelPath, err := resolveModelPath(modelPath)
	if err != nil {
		return nil, err
	}
	absLibPath, err := resolveORTSharedLibraryPath(libPath)
	if err != nil {
		return nil, err
	}
	libDir := filepath.Dir(absLibPath)
	prependPathEnv("PATH", libDir)
	configureORTSearchPath(libDir)
	setNativeEnv("ORT_LOGGING_LEVEL", "3")
	absCachePath, _ := filepath.Abs("trt_cache")
	_ = os.MkdirAll(absCachePath, 0755)
	configureTensorRTEnv(absCachePath)

	if !ort.IsInitialized() {
		ort.SetSharedLibraryPath(absLibPath)
		if err := ort.InitializeEnvironment(); err != nil {
			return nil, fmt.Errorf("initialize onnxruntime: %w", err)
		}
	}

	binInput := make([]float32, NumSpatialFeatures*minixiangqi.NumSquares)
	globalInput := make([]float32, NumGlobalFeatures)
	policy := make([]float32, PolicySize)
	value := make([]float32, 3)

	inputTensor1, err := ort.NewTensor(ort.NewShape(1, NumSpatialFeatures, minixiangqi.Rows, minixiangqi.Cols), binInput)
	if err != nil {
		return nil, err
	}
	inputTensor2, err := ort.NewTensor(ort.NewShape(1, NumGlobalFeatures), globalInput)
	if err != nil {
		inputTensor1.Destroy()
		return nil, err
	}
	outputTensor1, err := ort.NewTensor(ort.NewShape(1, PolicySize), policy)
	if err != nil {
		inputTensor2.Destroy()
		inputTensor1.Destroy()
		return nil, err
	}
	outputTensor2, err := ort.NewTensor(ort.NewShape(1, 3), value)
	if err != nil {
		outputTensor1.Destroy()
		inputTensor2.Destroy()
		inputTensor1.Destroy()
		return nil, err
	}

	session, providerName, err := newSessionWithProviders(
		absModelPath,
		[]ort.Value{inputTensor1, inputTensor2},
		[]ort.Value{outputTensor1, outputTensor2},
		absCachePath,
	)
	if err != nil {
		outputTensor2.Destroy()
		outputTensor1.Destroy()
		inputTensor2.Destroy()
		inputTensor1.Destroy()
		return nil, err
	}

	log.Printf("ONNX ready: %s (provider=%s)", absModelPath, providerName)
	nn := &NNEvaluator{
		session:     session,
		inputs:      []ort.Value{inputTensor1, inputTensor2},
		outputs:     []ort.Value{outputTensor1, outputTensor2},
		binInput:    binInput,
		globalInput: globalInput,
		policy:      policy,
		value:       value,
		provider:    providerName,
		modelPath:   absModelPath,
		cache:       newNNEvalCache(nnEvalCacheCapacity),
	}
	nn.StartAsyncWarmup(absCachePath)
	return nn, nil
}

func (n *NNEvaluator) Evaluate(pos *minixiangqi.Position) (*NNResult, error) {
	return n.EvaluateWithStage(pos, 0, -1)
}

func (n *NNEvaluator) EvaluateWithStage(pos *minixiangqi.Position, stage int, chosenSquare int) (*NNResult, error) {
	if n == nil || n.session == nil {
		return nil, errors.New("nn not initialized")
	}
	key := nnCacheKey{
		Hash:         pos.EnsureHash(),
		Stage:        int8(stage),
		ChosenSquare: int16(chosenSquare),
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if cached, ok := n.cache.Get(key); ok {
		return cached, nil
	}
	fillInputs(n.binInput, n.globalInput, pos, stage, chosenSquare)
	if err := n.session.Run(); err != nil {
		return nil, err
	}

	maxLogit := float64(n.value[0])
	for i := 1; i < 3; i++ {
		if float64(n.value[i]) > maxLogit {
			maxLogit = float64(n.value[i])
		}
	}
	e0 := math.Exp(float64(n.value[0]) - maxLogit)
	e1 := math.Exp(float64(n.value[1]) - maxLogit)
	e2 := math.Exp(float64(n.value[2]) - maxLogit)
	sum := e0 + e1 + e2
	if sum <= 0 {
		sum = 1
	}
	res := &NNResult{
		WinProb:  float32(e0 / sum),
		LossProb: float32(e1 / sum),
		DrawProb: float32(e2 / sum),
		Policy:   make([]float32, PolicySize),
	}
	copy(res.Policy, n.policy)
	n.cache.Put(key, res)
	return cloneNNResult(res), nil
}

func fillInputs(binInput []float32, globalInput []float32, pos *minixiangqi.Position, stage int, chosenSquare int) {
	for i := range binInput {
		binInput[i] = 0
	}
	for i := range globalInput {
		globalInput[i] = 0
	}

	planeSize := minixiangqi.NumSquares
	for sq := 0; sq < minixiangqi.NumSquares; sq++ {
		binInput[sq] = 1
	}

	for sq, piece := range pos.Board {
		if piece == minixiangqi.Empty {
			continue
		}
		base := featurePlaneForPiece(piece, pos.SideToMove)
		if base < 0 {
			continue
		}
		binInput[base*planeSize+sq] = 1
	}

	myRule73 := pos.Get73RuleHistory(pos.SideToMove, stage, chosenSquare, rule73Length(pos))
	for i, sq := range myRule73 {
		if sq >= 0 && sq < minixiangqi.NumSquares {
			binInput[(22+i)*planeSize+sq] = 1
		}
	}
	oppRule73 := pos.Get73RuleHistory(pos.SideToMove.Opponent(), stage, chosenSquare, rule73Length(pos))
	for i, sq := range oppRule73 {
		if sq >= 0 && sq < minixiangqi.NumSquares {
			binInput[(29+i)*planeSize+sq] = 1
		}
	}

	if stage == 1 && chosenSquare >= 0 && chosenSquare < minixiangqi.NumSquares {
		globalInput[0] = 1
		binInput[38*planeSize+chosenSquare] = 1
	}

	resultsBeforeNN := pos.GetResultsBeforeNN(stage, chosenSquare)
	if resultsBeforeNN.Inited {
		globalInput[1] = 1
		switch resultsBeforeNN.Winner {
		case -1:
			globalInput[2] = 1
		case int(pos.SideToMove):
			globalInput[3] = 1
		case int(pos.SideToMove.Opponent()):
			globalInput[4] = 1
		}
		if resultsBeforeNN.MyOnlyLoc >= 0 && resultsBeforeNN.MyOnlyLoc < minixiangqi.NumSquares {
			binInput[39*planeSize+resultsBeforeNN.MyOnlyLoc] = 1
		}
	}

	switch pos.Rules.ScoringRule {
	case minixiangqi.Scoring1:
		globalInput[6] = 1
	case minixiangqi.Scoring2:
		globalInput[7] = 1
	case minixiangqi.Scoring3:
		globalInput[6] = 1
		globalInput[7] = 1
	}

	switch pos.Rules.DrawJudgeRule {
	case minixiangqi.DrawJudgeCount:
		globalInput[26] = 1
	case minixiangqi.DrawJudgeWeight:
		globalInput[27] = 1
	}

	switch pos.Rules.LoopRule {
	case minixiangqi.LoopRuleNone:
		globalInput[28] = 1
	case minixiangqi.LoopRuleRepeatEnd:
		globalInput[29] = 1
	case minixiangqi.LoopRuleTwoOne:
		globalInput[30] = 1
	case minixiangqi.LoopRuleFiveTwo:
		globalInput[31] = 1
	}

	fillMoveLimitFeatures(globalInput, 8, pos.Rules.MaxMoves, pos.MoveNum)
	fillMoveLimitFeatures(globalInput, 15, pos.Rules.MaxMovesNoCapture, pos.MoveNumSinceCapture)
}

func featurePlaneForPiece(piece minixiangqi.Piece, sideToMove minixiangqi.Side) int {
	offset := 0
	if piece.Side() != sideToMove {
		offset = 5
	}
	switch piece.Type() {
	case minixiangqi.PiecePawn:
		return 1 + offset
	case minixiangqi.PieceRook:
		return 2 + offset
	case minixiangqi.PieceCannon:
		return 3 + offset
	case minixiangqi.PieceHorse:
		return 4 + offset
	case minixiangqi.PieceKing:
		return 5 + offset
	default:
		return -1
	}
}

func prependPathEnv(key, value string) {
	if value == "" {
		return
	}
	old := os.Getenv(key)
	if old == "" {
		setNativeEnv(key, value)
		return
	}
	setNativeEnv(key, value+string(os.PathListSeparator)+old)
}

type providerSpec struct {
	name  string
	setup func(*ort.SessionOptions) error
}

var warmupBatches = []int{1, 2, 4, 8, 16, 32}

func configureTensorRTEnv(cachePath string) {
	if cachePath == "" {
		return
	}
	setNativeEnv("ORT_TENSORRT_ENGINE_CACHE_ENABLE", "1")
	setNativeEnv("ORT_TENSORRT_ENGINE_CACHE_PATH", cachePath)
	setNativeEnv("ORT_TENSORRT_CACHE_ENABLE", "1")
	setNativeEnv("ORT_TENSORRT_CACHE_PATH", cachePath)
	setNativeEnv("ORT_TRT_ENGINE_CACHE_ENABLE", "1")
	setNativeEnv("ORT_TRT_CACHE_PATH", cachePath)
	setNativeEnv("ORT_TENSORRT_TIMING_CACHE_ENABLE", "1")
	setNativeEnv("ORT_TENSORRT_TIMING_CACHE_PATH", cachePath)
	setNativeEnv("ORT_TENSORRT_FP16_ENABLE", "1")
	setNativeEnv("ORT_TENSORRT_MAX_WORKSPACE_SIZE", "6442450944")
}

func windowsProviders(cachePath string) []providerSpec {
	return []providerSpec{
		{
			name: "TensorRT",
			setup: func(so *ort.SessionOptions) error {
				trtOpts, err := ort.NewTensorRTProviderOptions()
				if err != nil {
					return err
				}
				defer trtOpts.Destroy()
				if err := trtOpts.Update(map[string]string{
					"device_id":                      "0",
					"trt_engine_cache_enable":        "1",
					"trt_engine_cache_path":          cachePath,
					"trt_fp16_enable":                "1",
					"trt_max_workspace_size":         "6442450944",
					"trt_timing_cache_enable":        "1",
					"trt_timing_cache_path":          cachePath,
					"trt_builder_optimization_level": "4",
				}); err != nil {
					return err
				}
				return so.AppendExecutionProviderTensorRT(trtOpts)
			},
		},
		{
			name: "CUDA",
			setup: func(so *ort.SessionOptions) error {
				cudaOpts, err := ort.NewCUDAProviderOptions()
				if err != nil {
					return err
				}
				defer cudaOpts.Destroy()
				return so.AppendExecutionProviderCUDA(cudaOpts)
			},
		},
		{
			name: "DirectML",
			setup: func(so *ort.SessionOptions) error {
				return so.AppendExecutionProviderDirectML(0)
			},
		},
		{
			name: "CPU",
			setup: func(so *ort.SessionOptions) error {
				return nil
			},
		},
	}
}

func defaultProviders(cachePath string) []providerSpec {
	if runtime.GOOS == "windows" {
		return windowsProviders(cachePath)
	}
	return []providerSpec{
		{
			name: "CPU",
			setup: func(so *ort.SessionOptions) error {
				return nil
			},
		},
	}
}

func newSessionWithProviders(modelPath string, inputs []ort.Value, outputs []ort.Value, cachePath string) (*ort.AdvancedSession, string, error) {
	var lastErr error
	for _, spec := range defaultProviders(cachePath) {
		so, err := ort.NewSessionOptions()
		if err != nil {
			return nil, "", err
		}
		_ = so.SetLogSeverityLevel(3)
		if err := spec.setup(so); err != nil {
			so.Destroy()
			log.Printf("%s unavailable, trying next provider: %v", spec.name, err)
			lastErr = err
			continue
		}
		session, err := ort.NewAdvancedSession(
			modelPath,
			[]string{"bin_inputs", "global_inputs"},
			[]string{"policy", "value"},
			inputs,
			outputs,
			so,
		)
		so.Destroy()
		if err == nil {
			return session, spec.name, nil
		}
		log.Printf("%s session init failed, trying next provider: %v", spec.name, err)
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no execution provider available")
	}
	return nil, "", lastErr
}

func providerSpecByName(name, cachePath string) (providerSpec, bool) {
	for _, spec := range defaultProviders(cachePath) {
		if spec.name == name {
			return spec, true
		}
	}
	return providerSpec{}, false
}

func (n *NNEvaluator) StartAsyncWarmup(cachePath string) {
	if n == nil || n.modelPath == "" || n.provider == "" {
		return
	}
	spec, ok := providerSpecByName(n.provider, cachePath)
	if !ok {
		return
	}
	go func() {
		log.Printf("ONNX warmup started: provider=%s batches=%v", n.provider, warmupBatches)
		for _, batchSize := range warmupBatches {
			if err := warmupOneBatch(n.modelPath, spec, batchSize); err != nil {
				log.Printf("ONNX warmup batch=%d failed: %v", batchSize, err)
				return
			}
			log.Printf("ONNX warmup batch=%d ready", batchSize)
		}
		log.Printf("ONNX warmup finished: provider=%s", n.provider)
	}()
}

func warmupOneBatch(modelPath string, spec providerSpec, batchSize int) error {
	if batchSize <= 0 {
		return fmt.Errorf("invalid warmup batch size: %d", batchSize)
	}
	binInput := make([]float32, batchSize*NumSpatialFeatures*minixiangqi.NumSquares)
	globalInput := make([]float32, batchSize*NumGlobalFeatures)
	policy := make([]float32, batchSize*PolicySize)
	value := make([]float32, batchSize*3)

	inputTensor1, err := ort.NewTensor(
		ort.NewShape(int64(batchSize), NumSpatialFeatures, minixiangqi.Rows, minixiangqi.Cols),
		binInput,
	)
	if err != nil {
		return err
	}
	defer inputTensor1.Destroy()

	inputTensor2, err := ort.NewTensor(ort.NewShape(int64(batchSize), NumGlobalFeatures), globalInput)
	if err != nil {
		return err
	}
	defer inputTensor2.Destroy()

	outputTensor1, err := ort.NewTensor(ort.NewShape(int64(batchSize), PolicySize), policy)
	if err != nil {
		return err
	}
	defer outputTensor1.Destroy()

	outputTensor2, err := ort.NewTensor(ort.NewShape(int64(batchSize), 3), value)
	if err != nil {
		return err
	}
	defer outputTensor2.Destroy()

	so, err := ort.NewSessionOptions()
	if err != nil {
		return err
	}
	defer so.Destroy()
	_ = so.SetLogSeverityLevel(3)
	if err := spec.setup(so); err != nil {
		return err
	}

	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{"bin_inputs", "global_inputs"},
		[]string{"policy", "value"},
		[]ort.Value{inputTensor1, inputTensor2},
		[]ort.Value{outputTensor1, outputTensor2},
		so,
	)
	if err != nil {
		return err
	}
	defer session.Destroy()

	return session.Run()
}

func rule73Length(pos *minixiangqi.Position) int {
	switch pos.Rules.LoopRule {
	case minixiangqi.LoopRuleSeventhThree:
		return 7
	case minixiangqi.LoopRuleNone:
		return 0
	case minixiangqi.LoopRuleRepeatEnd:
		return 7
	case minixiangqi.LoopRuleTwoOne:
		return 2
	case minixiangqi.LoopRuleFiveTwo:
		return 5
	default:
		return 0
	}
}

func fillMoveLimitFeatures(globalInput []float32, base int, maxMoves int, moveNum int) {
	if maxMoves == 0 {
		return
	}
	globalInput[base] = 1
	dif := float64(maxMoves - moveNum)
	if dif < 0 {
		dif = 0
	}
	globalInput[base+1] = float32(math.Exp(-dif / 150.0))
	globalInput[base+2] = float32(math.Exp(-dif / 50.0))
	globalInput[base+3] = float32(math.Exp(-dif / 15.0))
	globalInput[base+4] = float32(math.Exp(-dif / 5.0))
	globalInput[base+5] = float32(math.Exp(-dif / 1.5))
	if int(dif)%2 == 0 {
		globalInput[base+6] = -1
	} else {
		globalInput[base+6] = 1
	}
}
