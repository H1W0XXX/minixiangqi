package httpserver

import "minixiangqi/internal/minixiangqi"

type MoveDTO struct {
	From int `json:"from"`
	To   int `json:"to"`
}

type NewGameRequest struct {
	ModelKey string `json:"model_key"`
}

type NewGameResponse struct {
	GameID     string    `json:"game_id"`
	ModelKey   string    `json:"model_key"`
	ModelLabel string    `json:"model_label"`
	Position   string    `json:"position"`
	ToMove     int       `json:"to_move"`
	LegalMoves []MoveDTO `json:"legal_moves"`
	InCheck    bool      `json:"in_check"`
	CheckFrom  []int     `json:"check_sources"`
	Status     string    `json:"status"`
}

type PlayRequest struct {
	GameID string  `json:"game_id"`
	Move   MoveDTO `json:"move"`
}

type PlayResponse struct {
	Position   string    `json:"position"`
	ToMove     int       `json:"to_move"`
	LegalMoves []MoveDTO `json:"legal_moves"`
	InCheck    bool      `json:"in_check"`
	CheckFrom  []int     `json:"check_sources"`
	Status     string    `json:"status"`
}

type StateRequest struct {
	GameID string `json:"game_id"`
}

type StateResponse struct {
	ModelKey   string    `json:"model_key"`
	ModelLabel string    `json:"model_label"`
	Position   string    `json:"position"`
	ToMove     int       `json:"to_move"`
	LegalMoves []MoveDTO `json:"legal_moves"`
	InCheck    bool      `json:"in_check"`
	CheckFrom  []int     `json:"check_sources"`
	Status     string    `json:"status"`
}

type UndoRequest struct {
	GameID string `json:"game_id"`
}

type AiMoveRequest struct {
	GameID          string `json:"game_id"`
	ModelKey        string `json:"model_key"`
	Position        string `json:"position"`
	ToMove          int    `json:"to_move"`
	TimeMs          int64  `json:"time_ms"`
	UseMCTS         bool   `json:"use_mcts"`
	MCTSSimulations int    `json:"mcts_simulations"`
}

type AiMoveResponse struct {
	BestMove   MoveDTO   `json:"best_move"`
	Score      int       `json:"score"`
	WinProb    float32   `json:"win_prob"`
	Nodes      int64     `json:"nodes"`
	TimeMs     int64     `json:"time_ms"`
	Position   string    `json:"position"`
	ToMove     int       `json:"to_move"`
	LegalMoves []MoveDTO `json:"legal_moves"`
	InCheck    bool      `json:"in_check"`
	CheckFrom  []int     `json:"check_sources"`
	Status     string    `json:"status"`
}

func movesToDTO(moves []minixiangqi.Move) []MoveDTO {
	out := make([]MoveDTO, len(moves))
	for i, mv := range moves {
		out[i] = MoveDTO{From: mv.From, To: mv.To}
	}
	return out
}

func sideToInt(side minixiangqi.Side) int {
	if side == minixiangqi.Black {
		return 1
	}
	return 0
}
