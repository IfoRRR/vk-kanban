package cases

import (
	"encoding/csv"
	"net/http"
	"strings"
)

func (h *Handler) uptimeAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/uptime/api/")
	switch {
	case r.Method == http.MethodGet && path == "state":
		writeJSON(w, http.StatusOK, map[string]any{
			"services": h.watch.Snapshot(r.URL.Query().Get("q"), r.URL.Query().Get("status")),
		})
	case r.Method == http.MethodGet && path == "service":
		item, err := h.watch.One(r.URL.Query().Get("id"))
		writeResult(w, item, err)
	case r.Method == http.MethodGet && path == "report":
		writeJSON(w, http.StatusOK, map[string]any{"services": h.watch.Snapshot("", "")})
	case r.Method == http.MethodGet && path == "report.csv":
		h.uptimeCSV(w)
	case r.Method == http.MethodPost && path == "service":
		var body struct{ Name, URL, Kind, Expect string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял сервис"})
			return
		}
		items, err := h.watch.Add(body.Name, body.URL, body.Kind, body.Expect)
		writeResult(w, map[string]any{"services": items}, err)
	case r.Method == http.MethodPost && path == "service/update":
		var body struct{ ID, Name, URL, Kind, Expect string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял правку"})
			return
		}
		items, err := h.watch.Update(body.ID, body.Name, body.URL, body.Kind, body.Expect)
		writeResult(w, map[string]any{"services": items}, err)
	case r.Method == http.MethodPost && path == "service/delete":
		var body struct{ ID string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял удаление"})
			return
		}
		items, err := h.watch.Delete(body.ID)
		writeResult(w, map[string]any{"services": items}, err)
	case r.Method == http.MethodPost && path == "check":
		writeJSON(w, http.StatusOK, map[string]any{"services": h.watch.CheckAll()})
	case r.Method == http.MethodPost && path == "reset":
		writeJSON(w, http.StatusOK, map[string]any{"services": h.watch.Reset()})
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) uptimeCSV(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=uptime.csv")
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	out := csv.NewWriter(w)
	_ = out.Write([]string{"сервис", "адрес", "доступность_%", "средний_мс", "проверок", "инцидентов", "сейчас"})
	for _, item := range h.watch.Snapshot("", "") {
		now := "нет данных"
		if item.Checked != "" {
			if item.OK {
				now = "отвечает"
			} else {
				now = "сбой"
			}
		}
		_ = out.Write([]string{item.Name, item.URL, itoa(item.Availability), itoa(item.AvgMS), itoa(item.Checks), itoa(len(item.Incidents)), now})
	}
	out.Flush()
}
