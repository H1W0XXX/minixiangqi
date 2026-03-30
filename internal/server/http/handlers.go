package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"minixiangqi/internal/engine"
	"minixiangqi/internal/minixiangqi"
)

type Game struct {
	Pos      *minixiangqi.Position
	History  []*minixiangqi.Position
	LastMove time.Time
	Engine   *engine.Engine
	ModelKey string
	ModelTag string
}

var (
	games       = make(map[string]*Game)
	gamesMu     sync.RWMutex
	gameSeq     uint64
	ai          = engine.NewEngine()
	models      = newModelCatalog()
	cleanupOnce sync.Once
)

type Handler struct{}

const (
	gameIdleTimeout  = 30 * time.Minute
	gameCleanupEvery = 5 * time.Minute
)

func NewHandler() *Handler {
	cleanupOnce.Do(startGameCleanupLoop)
	return &Handler{}
}

func (h *Handler) Engine() *engine.Engine { return ai }

func (h *Handler) ConfigureModels(modelPath, libPath, nnuePath, nnueSource string) {
	models.Configure(h.Engine().BackendName(), modelPath, libPath, nnuePath, nnueSource, h.Engine())
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/models":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleModels(w)
	case "/api/new_game":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleNewGame(w, r)
	case "/api/state":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleState(w, r)
	case "/api/play":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handlePlay(w, r)
	case "/api/undo":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleUndo(w, r)
	case "/api/ai_move":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleAIMove(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleModels(w http.ResponseWriter) {
	writeJSON(w, models.Snapshot())
}

func (h *Handler) handleNewGame(w http.ResponseWriter, r *http.Request) {
	var req NewGameRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
	}
	engineForGame, modelLabel, err := models.CloneEngineForKey(req.ModelKey)
	if err != nil {
		http.Error(w, "model not found", http.StatusBadRequest)
		return
	}
	if req.ModelKey == "" {
		req.ModelKey = models.Snapshot().CurrentKey
	}
	pos := minixiangqi.NewInitialPosition()
	game := &Game{
		Pos:      pos,
		History:  []*minixiangqi.Position{pos},
		LastMove: time.Now(),
		Engine:   engineForGame,
		ModelKey: req.ModelKey,
		ModelTag: modelLabel,
	}
	id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), atomic.AddUint64(&gameSeq, 1))
	gamesMu.Lock()
	games[id] = game
	gamesMu.Unlock()

	writeJSON(w, NewGameResponse{
		GameID:     id,
		ModelKey:   game.ModelKey,
		ModelLabel: game.ModelTag,
		Position:   pos.Encode(),
		ToMove:     sideToInt(pos.SideToMove),
		LegalMoves: movesToDTO(pos.GenerateLegalMoves()),
		InCheck:    pos.IsInCheck(pos.SideToMove),
		CheckFrom:  checkSourceSquares(pos),
		Status:     string(minixiangqi.EvaluateStatus(pos)),
	})
}

func (h *Handler) handleState(w http.ResponseWriter, r *http.Request) {
	var req StateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	game, err := touchGame(req.GameID)
	if err != nil {
		http.Error(w, "game not found", http.StatusNotFound)
		return
	}
	writeJSON(w, stateFromGame(game))
}

func (h *Handler) handlePlay(w http.ResponseWriter, r *http.Request) {
	var req PlayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	gamesMu.Lock()
	defer gamesMu.Unlock()

	game, ok := games[req.GameID]
	if !ok || game == nil || game.Pos == nil {
		http.Error(w, "game not found", http.StatusNotFound)
		return
	}
	mv := minixiangqi.Move{From: req.Move.From, To: req.Move.To}
	next, ok := game.Pos.ApplyMove(mv)
	if !ok {
		http.Error(w, "illegal move", http.StatusBadRequest)
		return
	}
	game.Pos = next
	game.History = append(game.History, next)
	game.LastMove = time.Now()
	writeJSON(w, stateFromGame(game))
}

func (h *Handler) handleUndo(w http.ResponseWriter, r *http.Request) {
	var req UndoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	gamesMu.Lock()
	defer gamesMu.Unlock()
	game, ok := games[req.GameID]
	if !ok || game == nil || len(game.History) <= 1 {
		http.Error(w, "cannot undo", http.StatusBadRequest)
		return
	}

	game.History = game.History[:len(game.History)-1]
	game.Pos = game.History[len(game.History)-1]
	game.LastMove = time.Now()
	writeJSON(w, stateFromGame(game))
}

func (h *Handler) handleAIMove(w http.ResponseWriter, r *http.Request) {
	var req AiMoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	game, err := touchGame(req.GameID)
	if err != nil {
		http.Error(w, "game not found", http.StatusNotFound)
		return
	}

	pos := game.Pos
	if req.Position != "" {
		if decoded, decErr := minixiangqi.DecodePosition(req.Position); decErr == nil {
			pos = decoded
		}
	}

	engineForMove := game.Engine
	if req.ModelKey != "" && req.ModelKey != game.ModelKey {
		overridden, _, cloneErr := models.CloneEngineForKey(req.ModelKey)
		if cloneErr != nil {
			http.Error(w, "model not found", http.StatusBadRequest)
			return
		}
		engineForMove = overridden
	}
	if engineForMove == nil {
		engineForMove = game.Engine
	}

	res := engineForMove.Search(pos, engine.SearchConfig{
		UseMCTS:         req.UseMCTS,
		MCTSSimulations: req.MCTSSimulations,
		MaxDepth:        req.MCTSSimulations,
		TimeLimit:       time.Duration(req.TimeMs) * time.Millisecond,
	})
	if res.NNFailed {
		http.Error(w, "nn failed", http.StatusInternalServerError)
		return
	}

	status := string(minixiangqi.StatusOngoing)
	if res.BestMove == (minixiangqi.Move{}) {
		status = string(minixiangqi.EvaluateStatus(pos))
	}
	writeJSON(w, AiMoveResponse{
		BestMove:   MoveDTO{From: res.BestMove.From, To: res.BestMove.To},
		Score:      res.Score,
		WinProb:    res.WinProb,
		Nodes:      res.Nodes,
		TimeMs:     res.TimeUsed.Milliseconds(),
		Position:   pos.Encode(),
		ToMove:     sideToInt(pos.SideToMove),
		LegalMoves: movesToDTO(pos.GenerateLegalMoves()),
		InCheck:    pos.IsInCheck(pos.SideToMove),
		CheckFrom:  checkSourceSquares(pos),
		Status:     status,
	})
}

func stateFromGame(game *Game) StateResponse {
	status := minixiangqi.EvaluateStatus(game.Pos)
	legalMoves := game.Pos.GenerateLegalMoves()
	return StateResponse{
		ModelKey:   game.ModelKey,
		ModelLabel: game.ModelTag,
		Position:   game.Pos.Encode(),
		ToMove:     sideToInt(game.Pos.SideToMove),
		LegalMoves: movesToDTO(legalMoves),
		InCheck:    game.Pos.IsInCheck(game.Pos.SideToMove),
		CheckFrom:  uniqueMoveSources(legalMoves),
		Status:     string(status),
	}
}

func checkSourceSquares(pos *minixiangqi.Position) []int {
	if pos == nil || !pos.IsInCheck(pos.SideToMove) {
		return nil
	}
	return uniqueMoveSources(pos.GenerateLegalMoves())
}

func uniqueMoveSources(moves []minixiangqi.Move) []int {
	if len(moves) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(moves))
	out := make([]int, 0, len(moves))
	for _, mv := range moves {
		if _, ok := seen[mv.From]; ok {
			continue
		}
		seen[mv.From] = struct{}{}
		out = append(out, mv.From)
	}
	return out
}

func getGame(id string) (*Game, error) {
	gamesMu.RLock()
	defer gamesMu.RUnlock()
	game, ok := games[id]
	if !ok || game == nil || game.Pos == nil {
		return nil, errors.New("game not found")
	}
	return game, nil
}

func touchGame(id string) (*Game, error) {
	gamesMu.Lock()
	defer gamesMu.Unlock()
	game, ok := games[id]
	if !ok || game == nil || game.Pos == nil {
		return nil, errors.New("game not found")
	}
	game.LastMove = time.Now()
	return game, nil
}

func startGameCleanupLoop() {
	ticker := time.NewTicker(gameCleanupEvery)
	go func() {
		for now := range ticker.C {
			cleanupExpiredGames(now)
		}
	}()
}

func cleanupExpiredGames(now time.Time) {
	cutoff := now.Add(-gameIdleTimeout)
	gamesMu.Lock()
	defer gamesMu.Unlock()
	for id, game := range games {
		if game == nil || game.Pos == nil || game.LastMove.Before(cutoff) {
			delete(games, id)
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
