package cases

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type Error string

func (e Error) Error() string { return string(e) }

const (
	errShort     = Error("Слишком коротко")
	errColumn    = Error("Нет такой колонки")
	errMissing   = Error("Не найдено")
	errDate      = Error("Выберите день")
	errSlot      = Error("Слот должен быть с 09:00 до 18:00, на 30, 60 или 90 минут")
	errBusy      = Error("Это время уже занято")
	errStore     = Error("Не удалось записать данные")
	errAuth      = Error("Нужны имя, почта и пароль от 6 символов")
	errTaken     = Error("Такая почта уже есть")
	errLogin     = Error("Почта или пароль не подошли")
	errRole      = Error("Роль: сотрудник или администратор")
	errForbidden = Error("Эту встречу может изменить только её автор")
	errRange     = Error("Возьмите период до 62 дней")
	errURL       = Error("Нужен публичный http(s) или страница этого сайта на порту 8081")
)

type Handler struct {
	log   *log.Logger
	board *BoardStore
	rooms *RoomStore
	watch *Monitor
	files map[string]http.Handler
	vue   http.Handler
}

func NewHandler(board *BoardStore, rooms *RoomStore, watch *Monitor, static map[string]string, vuePath string, logger *log.Logger) *Handler {
	files := map[string]http.Handler{}
	for prefix, dir := range static {
		files[prefix] = http.FileServer(http.Dir(dir))
	}
	return &Handler{
		log:   logger,
		board: board,
		rooms: rooms,
		watch: watch,
		files: files,
		vue: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, vuePath)
		}),
	}
}

func (h *Handler) Mount(mux *http.ServeMux) {
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("pong")) })
	mux.HandleFunc("/board/ws", h.boardWS)
	mux.HandleFunc("/board/api/", h.boardAPI)
	mux.HandleFunc("/rooms/api/", h.roomsAPI)
	mux.HandleFunc("/uptime/api/", h.uptimeAPI)
	for _, prefix := range []string{"/board/", "/rooms/", "/uptime/"} {
		mux.HandleFunc(prefix+"vue.global.prod.js", h.vue.ServeHTTP)
		dir := h.files[prefix]
		if dir == nil {
			continue
		}
		mux.Handle(prefix, http.StripPrefix(prefix, dir))
	}
}

func Serve(addr string, static map[string]string, vuePath, roomsFile, boardFile, watchFile string) error {
	logger := log.New(io.Discard, "", 0)
	logger = log.New(logWriter(), "[кейсы] ", log.LstdFlags)
	rooms, err := OpenRooms(roomsFile)
	if err != nil {
		return err
	}
	board, err := OpenBoard(boardFile)
	if err != nil {
		return err
	}
	watch, err := OpenMonitor(watchFile)
	if err != nil {
		return err
	}
	go func() {
		watch.CheckAll()
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			watch.CheckAll()
		}
	}()
	handler := NewHandler(board, rooms, watch, static, vuePath, logger)
	mux := http.NewServeMux()
	handler.Mount(mux)
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	logger.Printf("слушаю %s", addr)
	return server.ListenAndServe()
}

func logWriter() io.Writer { return log.Default().Writer() }

func writeResult(w http.ResponseWriter, state any, err error) {
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func readJSON(r *http.Request, dest any) error {
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, 32<<10))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func clean(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > limit {
		value = string(runes[:limit])
	}
	return strings.TrimSpace(value)
}

func fallback(value, alt string) string {
	if value == "" {
		return alt
	}
	return value
}

func newID() string {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf)
}
