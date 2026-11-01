package cases

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMonitorMathAndGuard(t *testing.T) {
	monitor := NewMonitor()
	if _, _, err := normalizeTarget("http://127.0.0.1:22/"); err != errURL {
		t.Fatalf("port: %v", err)
	}
	if _, _, err := normalizeTarget("http://169.254.169.254/latest"); err != errURL {
		t.Fatalf("metadata: %v", err)
	}
	if _, _, err := normalizeTarget("http://10.1.1.1/"); err != errURL {
		t.Fatalf("private: %v", err)
	}
	fetch, show, err := normalizeTarget("http://127.0.0.1:8081/quiz/ping")
	if err != nil || fetch != "http://127.0.0.1:8081/quiz/ping" || show != "/quiz/ping" {
		t.Fatalf("%v %s %s", err, fetch, show)
	}
	monitor.targets = []watchTarget{{ID: "a", Name: "Альфа", URL: fetch, Show: show, Kind: "http", Enabled: true}}
	monitor.record("a", Sample{At: time.Now().Add(-2 * time.Minute).Format(time.RFC3339), OK: true, MS: 12})
	monitor.record("a", Sample{At: time.Now().Add(-time.Minute).Format(time.RFC3339), OK: false, Error: "нет ответа"})
	monitor.record("a", Sample{At: time.Now().Format(time.RFC3339), OK: true, MS: 20})
	item, err := monitor.One("a")
	if err != nil {
		t.Fatal(err)
	}
	if item.Availability != 66 || item.AvgMS != 16 || len(item.Incidents) != 1 || item.Incidents[0].Closed == "" {
		t.Fatalf("%+v", item)
	}
	if _, err := monitor.Add("Чужой", "http://10.2.2.2/", "http", ""); err != errURL {
		t.Fatalf("add private: %v", err)
	}
	filtered := monitor.Snapshot("альфа", "up")
	if len(filtered) != 1 || filtered[0].Name != "Альфа" {
		t.Fatalf("%+v", filtered)
	}
}

func TestMonitorFileAndCSV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "uptime.json")
	monitor, err := OpenMonitor(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := monitor.Add("Пример", "http://127.0.0.1:8081/board/", "http", ""); err != nil {
		t.Fatal(err)
	}
	again, err := OpenMonitor(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range again.Snapshot("пример", "") {
		if item.Name == "Пример" && item.URL == "/board/" {
			found = true
		}
	}
	if !found {
		t.Fatal("target did not persist")
	}
	handler := NewHandler(NewBoard(), NewRooms(), again, nil, "vue.js", nil)
	response := httptest.NewRecorder()
	handler.uptimeAPI(response, httptest.NewRequest(http.MethodGet, "/uptime/api/report.csv", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "Пример") || !strings.Contains(response.Body.String(), "Сайт") {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
}
