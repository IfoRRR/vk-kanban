package cases

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type boardHub struct {
	mu   sync.Mutex
	subs map[string]map[chan []byte]struct{}
}

func (h *boardHub) join(id string) (chan []byte, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs == nil {
		h.subs = map[string]map[chan []byte]struct{}{}
	}
	if h.subs[id] == nil {
		h.subs[id] = map[chan []byte]struct{}{}
	}
	if len(h.subs[id]) >= 24 {
		return nil, false
	}
	ch := make(chan []byte, 4)
	h.subs[id][ch] = struct{}{}
	return ch, true
}

func (h *boardHub) leave(id string, ch chan []byte) {
	if ch == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.subs[id]
	if set == nil {
		return
	}
	if _, ok := set[ch]; !ok {
		return
	}
	delete(set, ch)
	close(ch)
}

func (h *boardHub) broadcast(id string, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[id] {
		select {
		case ch <- payload:
		default:
		}
	}
}

func (h *Handler) boardWS(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{CheckOrigin: sameOrigin, ReadBufferSize: 1024, WriteBufferSize: 2048}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(12 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		return
	}
	var hello struct {
		Board string `json:"board"`
		Token string `json:"token"`
	}
	if json.Unmarshal(data, &hello) != nil || hello.Board == "" {
		return
	}
	user, err := h.board.UserByToken(hello.Token)
	actor := Actor{}
	if err == nil {
		actor = Actor{ID: user.ID, Name: user.Name}
	}
	if !h.board.CanView(hello.Board, actor) {
		return
	}
	ch, ok := h.board.hub.join(hello.Board)
	if !ok {
		return
	}
	defer h.board.hub.leave(hello.Board, ch)
	_ = conn.SetReadDeadline(time.Time{})
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				h.board.hub.leave(hello.Board, ch)
				return
			}
		}
	}()
	for payload := range ch {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			return
		}
	}
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if httpserverHost(origin) == "" {
		return false
	}
	return strings.EqualFold(httpserverHost(origin), httpserverHost(r.Host))
}

func httpserverHost(value string) string {
	if strings.Contains(value, "://") {
		if i := strings.Index(value, "://"); i >= 0 {
			value = value[i+3:]
		}
		if i := strings.IndexAny(value, "/?#"); i >= 0 {
			value = value[:i]
		}
	}
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		return value
	}
	return host
}
