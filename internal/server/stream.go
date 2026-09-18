package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"wbo/internal/ir"
	"wbo/internal/runner"
)

// streamInterval 是推送流的取样间隔。规则服务把整局状态放在内存里，
// 所以"按 revision 变化推送"用轮询取样实现最简单：客户端拿到的是
// 幂等的完整状态，断开重连也不会丢状态（对局本身有 revision 与乐观锁）。
const streamInterval = 200 * time.Millisecond

// streamMatch 用 Server-Sent Events 推送对局状态。
//
//	event: state
//	data: {"matchId":"…","side":"own|oppo|spectator","revision":12,"state":{…},"events":[…]}
//
// 玩家凭据看到自己视角的状态，观战凭据看到双方手牌都隐藏的状态；
// 每 15 秒发一条注释行作为心跳。
func (s *Server) streamMatch(w http.ResponseWriter, r *http.Request, id string, room *match) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	room.mu.Lock()
	side, authorized := viewerFor(room, bearerToken(r))
	room.mu.Unlock()
	if !authorized {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	var lastRevision uint64
	haveRevision := false
	ticker := time.NewTicker(streamInterval)
	defer ticker.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
			continue
		case <-ticker.C:
		}
		room.mu.Lock()
		state, err := matchState(room, side)
		var events []ir.RuntimeEvent
		if room.session != nil {
			events = room.session.EventsFor(side)
		}
		bot, botError := room.botDriver != nil, room.botError
		room.mu.Unlock()
		if err != nil {
			continue
		}
		if haveRevision && state.Revision == lastRevision {
			continue
		}
		lastRevision, haveRevision = state.Revision, true
		payload := struct {
			MatchID  string            `json:"matchId"`
			Side     string            `json:"side"`
			Revision uint64            `json:"revision"`
			State    runner.StateView  `json:"state"`
			Events   []ir.RuntimeEvent `json:"events"`
			Bot      bool              `json:"bot,omitempty"`
			BotError string            `json:"botError,omitempty"`
		}{id, side, state.Revision, state, events, bot, botError}
		data, err := json.Marshal(payload)
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(w, "event: state\ndata: %s\n\n", data); err != nil {
			return
		}
		flusher.Flush()
	}
}
