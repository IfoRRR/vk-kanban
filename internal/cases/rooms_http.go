package cases

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strings"
)

func (h *Handler) roomsAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/rooms/api/")
	actor := h.actor(r)
	switch {
	case r.Method == http.MethodGet && path == "state":
		writeJSON(w, http.StatusOK, h.rooms.Snapshot(r.URL.Query().Get("date"), actor))
	case r.Method == http.MethodGet && path == "week":
		days, err := h.rooms.Week(r.URL.Query().Get("date"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"days": days})
	case r.Method == http.MethodGet && path == "free":
		minutes := atoi(r.URL.Query().Get("minutes"))
		rooms, err := h.rooms.Free(r.URL.Query().Get("date"), r.URL.Query().Get("start"), minutes)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"rooms": rooms})
	case r.Method == http.MethodGet && path == "report":
		report, err := h.rooms.Report(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
		writeResult(w, report, err)
	case r.Method == http.MethodGet && path == "report.csv":
		h.reportCSV(w, r)
	case r.Method == http.MethodGet && path == "calendar.ics":
		h.calendarICS(w, r)
	case r.Method == http.MethodPost && path == "book":
		var body struct {
			Date, Room, Start, Who, Title string
			Minutes                       int
		}
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял бронь"})
			return
		}
		state, err := h.rooms.Book(body.Date, body.Room, body.Start, body.Who, body.Title, body.Minutes, actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "update":
		var body struct {
			ID, Date, Room, Start, Title, View string
			Minutes                            int
		}
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял правку"})
			return
		}
		state, err := h.rooms.Update(body.ID, actor, body.Date, body.Room, body.Start, body.Title, body.View, body.Minutes)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "cancel":
		var body struct{ ID, Date string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял отмену"})
			return
		}
		state, err := h.rooms.Cancel(body.ID, body.Date, actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "reset":
		writeJSON(w, http.StatusOK, h.rooms.Reset())
	case r.Method == http.MethodPost && path == "register":
		var body struct{ Name, Email, Password, Role string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял регистрацию"})
			return
		}
		user, token, err := h.rooms.Register(body.Name, body.Email, body.Password, body.Role)
		writeSession(w, user, token, err)
	case r.Method == http.MethodPost && path == "login":
		var body struct{ Email, Password string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял вход"})
			return
		}
		user, token, err := h.rooms.Login(body.Email, body.Password)
		writeSession(w, user, token, err)
	case r.Method == http.MethodPost && path == "demo":
		var body struct{ Role string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял демо"})
			return
		}
		user, token, err := h.rooms.Demo(body.Role)
		writeSession(w, user, token, err)
	case r.Method == http.MethodPost && path == "logout":
		h.rooms.Logout(r.Header.Get("X-Rooms-Token"))
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) actor(r *http.Request) Actor {
	user, err := h.rooms.UserByToken(r.Header.Get("X-Rooms-Token"))
	if err != nil {
		return Actor{}
	}
	return Actor{ID: user.ID, Name: user.Name, Role: user.Role}
}

func (h *Handler) reportCSV(w http.ResponseWriter, r *http.Request) {
	report, err := h.rooms.Report(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=rooms.csv")
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	out := csv.NewWriter(w)
	_ = out.Write([]string{"комната", "встреч", "минут", "загрузка_%"})
	for _, room := range report.Rooms {
		_ = out.Write([]string{room.Name, itoa(room.Meetings), itoa(room.Minutes), itoa(room.Occupancy)})
	}
	out.Flush()
}

func (h *Handler) calendarICS(w http.ResponseWriter, r *http.Request) {
	events, err := h.rooms.Events(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=rooms.ics")
	names := map[string]string{}
	for _, room := range h.rooms.Snapshot(r.URL.Query().Get("from"), Actor{}).Rooms {
		names[room.ID] = room.Name
	}
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//portfolio//rooms//RU\r\n")
	for _, event := range events {
		room := names[event.Room]
		if room == "" {
			room = event.Room
		}
		start := icsStamp(event.Date, event.Start)
		endMin, _ := parseClock(event.Start)
		end := icsStamp(event.Date, formatClock(endMin+event.Minutes))
		fmt.Fprintf(&b, "BEGIN:VEVENT\r\nUID:%s-%s@rooms\r\nDTSTART:%s\r\nDTEND:%s\r\nSUMMARY:%s\r\nLOCATION:%s\r\nEND:VEVENT\r\n",
			event.ID, strings.ReplaceAll(event.Date, "-", ""), start, end, icsText(event.Title), icsText(room))
	}
	b.WriteString("END:VCALENDAR\r\n")
	_, _ = w.Write([]byte(b.String()))
}

func writeSession(w http.ResponseWriter, user PublicUser, token string, err error) {
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "token": token})
}

func atoi(value string) int {
	n := 0
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func itoa(value int) string {
	return fmt.Sprintf("%d", value)
}

func icsStamp(date, clock string) string {
	return strings.ReplaceAll(date, "-", "") + "T" + strings.ReplaceAll(clock, ":", "") + "00"
}

func icsText(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, ",", `\,`)
	value = strings.ReplaceAll(value, ";", `\;`)
	return value
}
