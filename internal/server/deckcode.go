package server

import (
	"encoding/json"
	"net/http"

	"wbo/internal/deckcode"
	"wbo/internal/runner"
)

// deckCodeRequest 既用于编码（给 cards）也用于解码（给 code）。
type deckCodeRequest struct {
	Cards  []int  `json:"cards,omitempty"`
	Format string `json:"format,omitempty"`
	Class  string `json:"class,omitempty"`
	Code   string `json:"code,omitempty"`
}

type deckCodeResponse struct {
	Code    string `json:"code,omitempty"`
	Cards   []int  `json:"cards,omitempty"`
	Format  string `json:"format,omitempty"`
	Class   string `json:"class,omitempty"`
	Legal   bool   `json:"legal"`
	Problem string `json:"problem,omitempty"`
}

// deckCodeHandler 提供官网 hash 式卡组码的编解码：
//
//	POST /api/deckcode {cards, format, class?}  → {code}
//	POST /api/deckcode {code}                   → {cards, format, class, legal, problem}
//
// 客户端只需要一个实现，避免前后端各写一套编码产生分歧。
func (s *Server) deckCodeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input deckCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if input.Code != "" {
		decoded, err := deckcode.Decode(input.Code)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		format, err := runner.FormatByID(s.cards, decoded.Format)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		out := deckCodeResponse{Code: input.Code, Cards: decoded.Cards, Format: decoded.Format, Class: decoded.Class, Legal: true}
		if err := runner.ValidateDeckForFormat(s.cards, decoded.Cards, format); err != nil {
			out.Legal, out.Problem = false, err.Error()
		}
		writeJSON(w, out)
		return
	}
	format, err := runner.FormatByID(s.cards, input.Format)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	class := input.Class
	if class == "" {
		class = deckcode.ClassForDeck(input.Cards, func(id int) string { return s.classOf(id) })
	}
	code, err := deckcode.Encode(format.ID, class, input.Cards)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out := deckCodeResponse{Code: code, Cards: append([]int(nil), input.Cards...), Format: format.ID, Class: class, Legal: true}
	if err := runner.ValidateDeckForFormat(s.cards, input.Cards, format); err != nil {
		out.Legal, out.Problem = false, err.Error()
	}
	writeJSON(w, out)
}

// classOf 返回卡牌的职业（卡池里没有这张卡时为空）。
func (s *Server) classOf(id int) string {
	for n := range s.cards.Cards {
		if s.cards.Cards[n].ID == id {
			return s.cards.Cards[n].Meta.Class
		}
	}
	return ""
}
