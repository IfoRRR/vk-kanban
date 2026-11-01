package cases

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateAndFreeRooms(t *testing.T) {
	rooms := NewRooms()
	free, err := rooms.Free("2026-10-26", "10:00", 60)
	if err != nil {
		t.Fatal(err)
	}
	for _, room := range free {
		if room.ID == "alpha" {
			t.Fatal("alpha is busy at 10:00")
		}
	}
	if len(free) != 3 {
		t.Fatalf("free: %+v", free)
	}
	state, err := rooms.Book("2026-10-26", "beta", "09:00", "Катя", "Свободно", 60, Actor{})
	if err != nil {
		t.Fatal(err)
	}
	id := ""
	for _, item := range state.Bookings {
		if item.Title == "Свободно" {
			id = item.ID
		}
	}
	if _, err := rooms.Update(id, Actor{}, "2026-10-26", "alpha", "10:00", "Перенос", "2026-10-26", 60); err != errBusy {
		t.Fatalf("move onto sprint: %v", err)
	}
	state, err = rooms.Update(id, Actor{}, "2026-10-26", "beta", "15:00", "Созвон", "2026-10-26", 30)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range state.Bookings {
		if item.ID == id && item.Start == "15:00" && item.Title == "Созвон" {
			found = true
		}
	}
	if !found {
		t.Fatalf("%+v", state.Bookings)
	}
}

func TestOwnerAndReport(t *testing.T) {
	rooms := NewRooms()
	owner, _, err := rooms.Register("Катя", "katya@example.com", "secret1", "employee")
	if err != nil {
		t.Fatal(err)
	}
	actor := Actor{ID: owner.ID, Name: owner.Name, Role: owner.Role}
	state, err := rooms.Book("2026-10-26", "gamma", "12:00", "чужое имя", "Разбор", 30, actor)
	if err != nil {
		t.Fatal(err)
	}
	id := ""
	for _, item := range state.Bookings {
		if item.Title == "Разбор" {
			id = item.ID
			if item.Who != "Катя" || item.Owner != owner.ID {
				t.Fatalf("%+v", item)
			}
		}
	}
	other, _, err := rooms.Register("Илья", "ilya@example.com", "secret1", "employee")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rooms.Cancel(id, "2026-10-26", Actor{ID: other.ID, Name: other.Name, Role: other.Role}); err != errForbidden {
		t.Fatalf("other: %v", err)
	}
	if _, err := rooms.Cancel(id, "2026-10-26", Actor{ID: owner.ID, Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	report, err := rooms.Report("2026-10-26", "2026-10-26")
	if err != nil {
		t.Fatal(err)
	}
	if report.Rooms[0].Name != "Альфа" || report.Rooms[0].Minutes != 60 || report.Rooms[0].Occupancy != 11 {
		t.Fatalf("%+v", report.Rooms[0])
	}
	days, err := rooms.Week("2026-10-26")
	if err != nil || len(days) != 7 || !strings.HasPrefix(days[0].Label, "пн") {
		t.Fatalf("%v %+v", err, days)
	}
}

func TestRoomsFileAndCSV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rooms.json")
	store, err := OpenRooms(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Book("2026-10-27", "beta", "09:00", "Катя", "План", 30, Actor{}); err != nil {
		t.Fatal(err)
	}
	again, err := OpenRooms(path)
	if err != nil {
		t.Fatal(err)
	}
	state := again.Snapshot("2026-10-27", Actor{})
	found := false
	for _, item := range state.Bookings {
		if item.Title == "План" {
			found = true
		}
	}
	if !found {
		t.Fatal("booking did not persist")
	}
	handler := NewHandler(NewBoard(), again, NewMonitor(), nil, "vue.js", nil)
	request := httptest.NewRequest(http.MethodGet, "/rooms/api/report.csv?from=2026-10-27&to=2026-10-27", nil)
	response := httptest.NewRecorder()
	handler.roomsAPI(response, request)
	body := response.Body.String()
	if response.Code != 200 || !strings.Contains(body, "Бета") || !strings.Contains(body, "Альфа") {
		t.Fatalf("%d %s", response.Code, body)
	}
	request = httptest.NewRequest(http.MethodGet, "/rooms/api/calendar.ics?from=2026-10-27&to=2026-10-27", nil)
	response = httptest.NewRecorder()
	handler.roomsAPI(response, request)
	if !strings.Contains(response.Body.String(), "BEGIN:VEVENT") || !strings.Contains(response.Body.String(), "План") {
		t.Fatal(response.Body.String())
	}
}
