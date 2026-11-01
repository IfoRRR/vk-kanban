package cases

import "testing"

func cardByID(state BoardState, id string) *Card {
	for i := range state.Board.Cards {
		if state.Board.Cards[i].ID == id {
			return &state.Board.Cards[i]
		}
	}
	return nil
}

func TestMoveAndComment(t *testing.T) {
	board := NewBoard()
	state, err := board.Move("demo", "c1", "work", "", Actor{})
	if err != nil {
		t.Fatal(err)
	}
	card := cardByID(state, "c1")
	if card == nil || card.Column != "work" || len(state.Board.Events) != 1 {
		t.Fatalf("%+v", state.Board)
	}
	state, err = board.Comment("demo", "c1", "  взял в работу  ", Actor{})
	card = cardByID(state, "c1")
	if err != nil || card == nil || card.Comments[0].Text != "взял в работу" {
		t.Fatalf("%v %+v", err, card)
	}
	if _, err := board.Move("demo", "c1", "nope", "", Actor{}); err != errColumn {
		t.Fatal(err)
	}
}

func TestAddCardChoosesColumn(t *testing.T) {
	board := NewBoard()
	state, err := board.AddCard("demo", "Собрать отчёт", "коротко", "Катя", "2026-11-02", "отчёт", "done", Actor{})
	if err != nil {
		t.Fatal(err)
	}
	found := cardByTitle(state, "Собрать отчёт")
	if found == nil || found.Column != "done" || found.Assignee != "Катя" || found.Due != "2026-11-02" || len(found.Tags) != 1 {
		t.Fatalf("%+v", found)
	}
	state, err = board.AddCard("demo", "Без колонки", "", "", "", "", "missing", Actor{})
	if err != nil {
		t.Fatal(err)
	}
	found = cardByTitle(state, "Без колонки")
	if found == nil || found.Column != "backlog" {
		t.Fatalf("%+v", found)
	}
}

func cardByTitle(state BoardState, title string) *Card {
	for i := range state.Board.Cards {
		if state.Board.Cards[i].Title == title {
			return &state.Board.Cards[i]
		}
	}
	return nil
}

func TestRoomRejectsOverlap(t *testing.T) {
	rooms := NewRooms()
	if _, err := rooms.Book("2026-10-26", "alpha", "10:30", "Катя", "Пересечение", 30, Actor{}); err != errBusy {
		t.Fatalf("overlap: %v", err)
	}
	state, err := rooms.Book("2026-10-26", "beta", "09:00", "Катя", "Свободно", 60, Actor{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range state.Bookings {
		if item.Room == "beta" && item.Start == "09:00" {
			found = true
		}
	}
	if !found {
		t.Fatalf("%+v", state.Bookings)
	}
	if _, err := rooms.Book("2026-10-26", "beta", "18:00", "Катя", "Поздно", 30, Actor{}); err != errSlot {
		t.Fatalf("late: %v", err)
	}
}
