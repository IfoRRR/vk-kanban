package cases

import (
	"net/http"
	"strings"
)

func (h *Handler) boardAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/board/api/")
	actor := h.boardActor(r)
	switch {
	case r.Method == http.MethodGet && path == "state":
		state, err := h.board.Snapshot(r.URL.Query().Get("board"), actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "register":
		var body struct{ Name, Email, Password string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял регистрацию"})
			return
		}
		user, token, err := h.board.Register(body.Name, body.Email, body.Password)
		writeSession(w, user, token, err)
	case r.Method == http.MethodPost && path == "login":
		var body struct{ Email, Password string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял вход"})
			return
		}
		user, token, err := h.board.Login(body.Email, body.Password)
		writeSession(w, user, token, err)
	case r.Method == http.MethodPost && path == "demo":
		var body struct{ Kind string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял демо"})
			return
		}
		user, token, err := h.board.Demo(body.Kind)
		writeSession(w, user, token, err)
	case r.Method == http.MethodPost && path == "logout":
		h.board.Logout(r.Header.Get("X-Board-Token"))
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case r.Method == http.MethodPost && path == "create":
		var body struct{ Title, Template string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял доску"})
			return
		}
		state, err := h.board.Create(body.Title, body.Template, actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "rename":
		var body struct{ Board, Title string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял название"})
			return
		}
		state, err := h.board.RenameBoard(body.Board, body.Title, actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "delete":
		var body struct{ Board string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял удаление"})
			return
		}
		state, err := h.board.Delete(body.Board, actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "invite":
		var body struct{ Board, Email string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял участника"})
			return
		}
		state, err := h.board.Invite(body.Board, body.Email, actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "column":
		var body struct{ Board, Title string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял колонку"})
			return
		}
		state, err := h.board.AddColumn(body.Board, body.Title, actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "column/rename":
		var body struct{ Board, ID, Title string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял колонку"})
			return
		}
		state, err := h.board.RenameColumn(body.Board, body.ID, body.Title, actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "card":
		var body struct {
			Board, Title, Description, Assignee, Due, Tag, Column string
		}
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял карточку"})
			return
		}
		state, err := h.board.AddCard(body.Board, body.Title, body.Description, body.Assignee, body.Due, body.Tag, body.Column, actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "card/update":
		var body struct {
			Board, ID, Title, Description, Assignee, Due, Tags string
		}
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял правку"})
			return
		}
		state, err := h.board.UpdateCard(body.Board, body.ID, body.Title, body.Description, body.Assignee, body.Due, body.Tags, actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "card/delete":
		var body struct{ Board, ID string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял удаление"})
			return
		}
		state, err := h.board.DeleteCard(body.Board, body.ID, actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "move":
		var body struct{ Board, ID, Column, Before string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял перенос"})
			return
		}
		state, err := h.board.Move(body.Board, body.ID, body.Column, body.Before, actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "comment":
		var body struct{ Board, ID, Text string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял комментарий"})
			return
		}
		state, err := h.board.Comment(body.Board, body.ID, body.Text, actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "check":
		var body struct{ Board, ID, Text string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял пункт"})
			return
		}
		state, err := h.board.AddCheck(body.Board, body.ID, body.Text, actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "check/toggle":
		var body struct{ Board, ID, Check string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял пункт"})
			return
		}
		state, err := h.board.ToggleCheck(body.Board, body.ID, body.Check, actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "link":
		var body struct{ Board, ID, Name, URL string }
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Не понял вложение"})
			return
		}
		state, err := h.board.AddLink(body.Board, body.ID, body.Name, body.URL, actor)
		writeResult(w, state, err)
	case r.Method == http.MethodPost && path == "reset":
		writeJSON(w, http.StatusOK, h.board.Reset())
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) boardActor(r *http.Request) Actor {
	user, err := h.board.UserByToken(r.Header.Get("X-Board-Token"))
	if err != nil {
		return Actor{}
	}
	return Actor{ID: user.ID, Name: user.Name, Role: user.Role}
}
