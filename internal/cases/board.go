package cases

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Comment struct {
	ID     string `json:"id"`
	Author string `json:"author"`
	Text   string `json:"text"`
	At     string `json:"at"`
}

type Check struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type Link struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Card struct {
	ID          string    `json:"id"`
	Column      string    `json:"column"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Assignee    string    `json:"assignee"`
	Due         string    `json:"due"`
	Pos         int       `json:"pos"`
	Tags        []string  `json:"tags"`
	Comments    []Comment `json:"comments"`
	Checks      []Check   `json:"checks"`
	Links       []Link    `json:"links"`
}

type Event struct {
	At   string `json:"at"`
	Text string `json:"text"`
}

type Column struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type Member struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type BoardBody struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Public  bool     `json:"public"`
	Owner   string   `json:"owner"`
	OwnerID string   `json:"ownerId,omitempty"`
	Members []Member `json:"members"`
	Columns []Column `json:"columns"`
	Cards   []Card   `json:"cards"`
	Events  []Event  `json:"events"`
}

type BoardRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Role  string `json:"role"`
}

type BoardState struct {
	Board  BoardBody   `json:"board"`
	Boards []BoardRef  `json:"boards"`
	User   *PublicUser `json:"user,omitempty"`
}

type boardRec struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Public    bool     `json:"public"`
	Owner     string   `json:"owner"`
	MemberIDs []string `json:"members"`
	Columns   []Column `json:"columns"`
	Cards     []Card   `json:"cards"`
	Events    []Event  `json:"events"`
}

type boardUser struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Salt    string `json:"salt"`
	Pass    string `json:"pass"`
	Token   string `json:"token,omitempty"`
	Created int64  `json:"created"`
}

type boardDisk struct {
	Users  []boardUser `json:"users"`
	Boards []boardRec  `json:"boards"`
}

type BoardStore struct {
	mu     sync.Mutex
	path   string
	users  []boardUser
	boards []boardRec
	hub    boardHub
}

func NewBoard() *BoardStore {
	return &BoardStore{boards: []boardRec{seedDemo()}}
}

func OpenBoard(path string) (*BoardStore, error) {
	store := NewBoard()
	if path == "" {
		return store, nil
	}
	store.path = path
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil && len(strings.TrimSpace(string(data))) > 0 {
		var disk boardDisk
		if err := json.Unmarshal(data, &disk); err != nil {
			return nil, err
		}
		store.users = disk.Users
		if len(disk.Boards) > 0 {
			store.boards = disk.Boards
		}
	}
	store.ensureDemoLocked()
	store.ensurePeopleLocked()
	return store, store.saveLocked()
}

func seedDemo() boardRec {
	return boardRec{
		ID: "demo", Title: "Общая доска", Public: true,
		Columns: []Column{
			{ID: "backlog", Title: "Бэклог"},
			{ID: "work", Title: "В работе"},
			{ID: "done", Title: "Готово"},
		},
		Cards: []Card{
			{ID: "c1", Column: "backlog", Pos: 0, Title: "Согласовать колонки", Description: "Название колонки меняется на месте.", Assignee: "Аня", Due: "2026-10-28", Tags: []string{"процесс"}, Comments: []Comment{}, Checks: []Check{}, Links: []Link{}},
			{ID: "c2", Column: "work", Pos: 0, Title: "Перетащить карточку", Description: "Порядок внутри колонки сохраняется.", Assignee: "Илья", Due: "2026-10-26", Tags: []string{"интерфейс"}, Comments: []Comment{{ID: "m1", Author: "Илья", Text: "Нужны кнопки, не только мышь", At: "09:00"}}, Checks: []Check{{ID: "k1", Text: "Ручка перетаскивания", Done: false}}, Links: []Link{}},
			{ID: "c3", Column: "done", Pos: 0, Title: "Открыть демо без регистрации", Description: "Общая доска доступна сразу.", Assignee: "Катя", Due: "2026-10-25", Tags: []string{"демо"}, Comments: []Comment{}, Checks: []Check{}, Links: []Link{{Name: "Кейсы", URL: "/#cases"}}},
		},
	}
}

func (s *BoardStore) Snapshot(boardID string, actor Actor) (BoardState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.viewLocked(boardID, actor)
}

func (s *BoardStore) CanView(boardID string, actor Actor) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	board := s.find(boardID)
	return board != nil && canView(board, actor)
}

func (s *BoardStore) Register(name, email, password string) (PublicUser, string, error) {
	label := clean(name, 24)
	if len([]rune(label)) < 2 {
		return PublicUser{}, "", errShort
	}
	mail := roomEmail(email)
	if mail == "" || len(password) < 6 || len(password) > 72 {
		return PublicUser{}, "", errAuth
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.users) >= 200 {
		return PublicUser{}, "", Error("Регистрация сейчас закрыта")
	}
	for _, user := range s.users {
		if user.Email == mail {
			return PublicUser{}, "", errTaken
		}
	}
	salt := newSecret()
	user := boardUser{ID: labelID(), Name: label, Email: mail, Salt: salt, Pass: stretch(password, salt), Token: newSecret(), Created: time.Now().Unix()}
	s.users = append(s.users, user)
	if err := s.saveLocked(); err != nil {
		return PublicUser{}, "", errStore
	}
	return user.public(), user.Token, nil
}

func (s *BoardStore) Login(email, password string) (PublicUser, string, error) {
	mail := roomEmail(email)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, user := range s.users {
		if user.Email != mail {
			continue
		}
		if user.Pass != "" && subtlePass(user.Pass, stretch(password, user.Salt)) {
			s.users[i].Token = newSecret()
			if err := s.saveLocked(); err != nil {
				return PublicUser{}, "", errStore
			}
			return s.users[i].public(), s.users[i].Token, nil
		}
		return PublicUser{}, "", errLogin
	}
	return PublicUser{}, "", errLogin
}

func (s *BoardStore) Demo(kind string) (PublicUser, string, error) {
	email := "owner@demo.local"
	if kind == "member" {
		email = "member@demo.local"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensurePeopleLocked()
	for i, user := range s.users {
		if user.Email != email {
			continue
		}
		s.users[i].Token = newSecret()
		if err := s.saveLocked(); err != nil {
			return PublicUser{}, "", errStore
		}
		return s.users[i].public(), s.users[i].Token, nil
	}
	return PublicUser{}, "", errMissing
}

func (s *BoardStore) Logout(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, user := range s.users {
		if sameSecret(user.Token, token) {
			s.users[i].Token = ""
			_ = s.saveLocked()
			return
		}
	}
}

func (s *BoardStore) UserByToken(token string) (PublicUser, error) {
	if token == "" {
		return PublicUser{}, errLogin
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, user := range s.users {
		if sameSecret(user.Token, token) {
			return user.public(), nil
		}
	}
	return PublicUser{}, errLogin
}

func (s *BoardStore) Create(title, template string, actor Actor) (BoardState, error) {
	title = clean(title, 60)
	if len([]rune(title)) < 2 {
		return BoardState{}, errShort
	}
	if actor.ID == "" {
		return BoardState{}, errForbidden
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.boards) >= 80 {
		return BoardState{}, Error("Слишком много досок")
	}
	board := boardRec{ID: "d" + newID(), Title: title, Owner: actor.ID, Columns: templateColumns(template), Cards: []Card{}}
	s.boards = append(s.boards, board)
	if err := s.saveLocked(); err != nil {
		return BoardState{}, errStore
	}
	s.hub.broadcast(board.ID, ping(board.ID))
	return s.viewLocked(board.ID, actor)
}

func (s *BoardStore) RenameBoard(boardID, title string, actor Actor) (BoardState, error) {
	title = clean(title, 60)
	if len([]rune(title)) < 2 {
		return BoardState{}, errShort
	}
	return s.edit(boardID, actor, true, func(board *boardRec) error {
		board.Title = title
		s.note(board, "Доска переименована в «"+title+"»")
		return nil
	})
}

func (s *BoardStore) Delete(boardID string, actor Actor) (BoardState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	board := s.find(boardID)
	if board == nil {
		return BoardState{}, errMissing
	}
	if board.Public || board.Owner != actor.ID {
		return BoardState{}, errForbidden
	}
	next := s.boards[:0]
	for _, item := range s.boards {
		if item.ID != boardID {
			next = append(next, item)
		}
	}
	s.boards = next
	if err := s.saveLocked(); err != nil {
		return BoardState{}, errStore
	}
	s.hub.broadcast(boardID, ping(boardID))
	return s.viewLocked("demo", actor)
}

func (s *BoardStore) Invite(boardID, email string, actor Actor) (BoardState, error) {
	mail := roomEmail(email)
	if mail == "" {
		return BoardState{}, errAuth
	}
	return s.edit(boardID, actor, true, func(board *boardRec) error {
		var person *boardUser
		for i := range s.users {
			if s.users[i].Email == mail {
				person = &s.users[i]
				break
			}
		}
		if person == nil {
			return errMissing
		}
		if person.ID == board.Owner || contains(board.MemberIDs, person.ID) {
			return nil
		}
		if len(board.MemberIDs) >= 20 {
			return Error("На доске больше нет места для участников")
		}
		board.MemberIDs = append(board.MemberIDs, person.ID)
		s.note(board, person.Name+" в участниках")
		return nil
	})
}

func (s *BoardStore) AddColumn(boardID, title string, actor Actor) (BoardState, error) {
	title = clean(title, 32)
	if len([]rune(title)) < 2 {
		return BoardState{}, errShort
	}
	return s.edit(boardID, actor, false, func(board *boardRec) error {
		if len(board.Columns) >= 8 {
			return Error("Колонок уже достаточно")
		}
		board.Columns = append(board.Columns, Column{ID: "l" + newID(), Title: title})
		s.note(board, "Колонка «"+title+"»")
		return nil
	})
}

func (s *BoardStore) RenameColumn(boardID, column, title string, actor Actor) (BoardState, error) {
	title = clean(title, 32)
	if len([]rune(title)) < 2 {
		return BoardState{}, errShort
	}
	return s.edit(boardID, actor, false, func(board *boardRec) error {
		for i := range board.Columns {
			if board.Columns[i].ID == column {
				board.Columns[i].Title = title
				s.note(board, "Колонка теперь «"+title+"»")
				return nil
			}
		}
		return errColumn
	})
}

func (s *BoardStore) AddCard(boardID, title, description, assignee, due, tag, column string, actor Actor) (BoardState, error) {
	title = clean(title, 80)
	if len([]rune(title)) < 2 {
		return BoardState{}, errShort
	}
	return s.edit(boardID, actor, false, func(board *boardRec) error {
		if len(board.Columns) == 0 {
			return errColumn
		}
		if len(board.Cards) >= 200 {
			return Error("На доске больше нет места")
		}
		target := board.Columns[0].ID
		if column != "" && columnTitle(board, column) != "" {
			target = column
		}
		card := Card{
			ID: "c" + newID(), Column: target, Pos: nextPos(board, target), Title: title,
			Description: clean(description, 400), Assignee: fallback(clean(assignee, 24), guestName(actor)),
			Due: clean(due, 10), Tags: splitTags(tag), Comments: []Comment{}, Checks: []Check{}, Links: []Link{},
		}
		if !validDate(card.Due) {
			card.Due = ""
		}
		board.Cards = append(board.Cards, card)
		s.note(board, "Новая карточка «"+card.Title+"»")
		return nil
	})
}

func (s *BoardStore) UpdateCard(boardID, cardID, title, description, assignee, due, tags string, actor Actor) (BoardState, error) {
	title = clean(title, 80)
	if len([]rune(title)) < 2 {
		return BoardState{}, errShort
	}
	return s.edit(boardID, actor, false, func(board *boardRec) error {
		card := findCard(board, cardID)
		if card == nil {
			return errMissing
		}
		card.Title = title
		card.Description = clean(description, 400)
		card.Assignee = fallback(clean(assignee, 24), card.Assignee)
		card.Due = clean(due, 10)
		if !validDate(card.Due) {
			card.Due = ""
		}
		card.Tags = splitTags(tags)
		s.note(board, "Правка «"+card.Title+"»")
		return nil
	})
}

func (s *BoardStore) DeleteCard(boardID, cardID string, actor Actor) (BoardState, error) {
	return s.edit(boardID, actor, false, func(board *boardRec) error {
		next := board.Cards[:0]
		found := false
		title := ""
		for _, card := range board.Cards {
			if card.ID == cardID {
				found = true
				title = card.Title
				continue
			}
			next = append(next, card)
		}
		if !found {
			return errMissing
		}
		board.Cards = next
		s.note(board, "Удалена «"+title+"»")
		return nil
	})
}

func (s *BoardStore) Move(boardID, cardID, column, before string, actor Actor) (BoardState, error) {
	return s.edit(boardID, actor, false, func(board *boardRec) error {
		if columnTitle(board, column) == "" {
			return errColumn
		}
		card := findCard(board, cardID)
		if card == nil {
			return errMissing
		}
		card.Column = column
		if before == "" || before == cardID {
			card.Pos = nextPos(board, column)
		} else {
			target := findCard(board, before)
			if target == nil || target.Column != column {
				card.Pos = nextPos(board, column)
			} else {
				card.Pos = target.Pos
				for i := range board.Cards {
					other := &board.Cards[i]
					if other.ID != card.ID && other.Column == column && other.Pos >= card.Pos {
						other.Pos++
					}
				}
			}
		}
		s.note(board, "«"+card.Title+"» → "+columnTitle(board, column))
		return nil
	})
}

func (s *BoardStore) Comment(boardID, cardID, text string, actor Actor) (BoardState, error) {
	text = clean(text, 200)
	if text == "" {
		return BoardState{}, errShort
	}
	return s.edit(boardID, actor, false, func(board *boardRec) error {
		card := findCard(board, cardID)
		if card == nil {
			return errMissing
		}
		if len(card.Comments) >= 40 {
			return errShort
		}
		card.Comments = append(card.Comments, Comment{ID: "m" + newID(), Author: guestName(actor), Text: text, At: time.Now().Format("15:04")})
		s.note(board, "Комментарий к «"+card.Title+"»")
		return nil
	})
}

func (s *BoardStore) AddCheck(boardID, cardID, text string, actor Actor) (BoardState, error) {
	text = clean(text, 80)
	if text == "" {
		return BoardState{}, errShort
	}
	return s.edit(boardID, actor, false, func(board *boardRec) error {
		card := findCard(board, cardID)
		if card == nil {
			return errMissing
		}
		if len(card.Checks) >= 20 {
			return errShort
		}
		card.Checks = append(card.Checks, Check{ID: "k" + newID(), Text: text})
		s.note(board, "Пункт в «"+card.Title+"»")
		return nil
	})
}

func (s *BoardStore) ToggleCheck(boardID, cardID, checkID string, actor Actor) (BoardState, error) {
	return s.edit(boardID, actor, false, func(board *boardRec) error {
		card := findCard(board, cardID)
		if card == nil {
			return errMissing
		}
		for i := range card.Checks {
			if card.Checks[i].ID == checkID {
				card.Checks[i].Done = !card.Checks[i].Done
				return nil
			}
		}
		return errMissing
	})
}

func (s *BoardStore) AddLink(boardID, cardID, name, raw string, actor Actor) (BoardState, error) {
	url, ok := safeURL(raw)
	if !ok {
		return BoardState{}, errShort
	}
	name = fallback(clean(name, 40), "Ссылка")
	return s.edit(boardID, actor, false, func(board *boardRec) error {
		card := findCard(board, cardID)
		if card == nil {
			return errMissing
		}
		if len(card.Links) >= 8 {
			return errShort
		}
		card.Links = append(card.Links, Link{Name: name, URL: url})
		s.note(board, "Вложение к «"+card.Title+"»")
		return nil
	})
}

func (s *BoardStore) Reset() BoardState {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := []boardRec{seedDemo()}
	for _, board := range s.boards {
		if board.ID != "demo" {
			next = append(next, board)
		}
	}
	s.boards = next
	_ = s.saveLocked()
	s.hub.broadcast("demo", ping("demo"))
	return mustView(s.viewLocked("demo", Actor{}))
}

func (s *BoardStore) edit(boardID string, actor Actor, ownerOnly bool, fn func(*boardRec) error) (BoardState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	board := s.find(boardID)
	if board == nil {
		return BoardState{}, errMissing
	}
	if ownerOnly {
		if board.Public || board.Owner != actor.ID {
			return BoardState{}, errForbidden
		}
	} else if !canEdit(board, actor) {
		return BoardState{}, errForbidden
	}
	backup := cloneRec(*board)
	if err := fn(board); err != nil {
		*board = backup
		return BoardState{}, err
	}
	if err := s.saveLocked(); err != nil {
		return BoardState{}, errStore
	}
	s.hub.broadcast(board.ID, ping(board.ID))
	return s.viewLocked(board.ID, actor)
}

func (s *BoardStore) viewLocked(boardID string, actor Actor) (BoardState, error) {
	if boardID == "" {
		boardID = "demo"
	}
	board := s.find(boardID)
	if board == nil || !canView(board, actor) {
		return BoardState{}, errForbidden
	}
	refs := []BoardRef{}
	for i := range s.boards {
		item := &s.boards[i]
		if !canView(item, actor) {
			continue
		}
		role := "гость"
		if item.Public {
			role = "общая"
		} else if item.Owner == actor.ID {
			role = "владелец"
		} else if contains(item.MemberIDs, actor.ID) {
			role = "участник"
		}
		refs = append(refs, BoardRef{ID: item.ID, Title: item.Title, Role: role})
	}
	return BoardState{Board: s.bodyLocked(board), Boards: refs, User: s.publicLocked(actor.ID)}, nil
}

func (s *BoardStore) bodyLocked(board *boardRec) BoardBody {
	body := BoardBody{
		ID: board.ID, Title: board.Title, Public: board.Public, OwnerID: board.Owner,
		Owner: s.userName(board.Owner), Members: []Member{}, Columns: append([]Column(nil), board.Columns...),
		Cards: append([]Card(nil), board.Cards...), Events: append([]Event(nil), board.Events...),
	}
	if body.Owner == "" && board.Public {
		body.Owner = "Общая"
	}
	for _, id := range board.MemberIDs {
		body.Members = append(body.Members, Member{ID: id, Name: s.userName(id)})
	}
	for i := range body.Cards {
		body.Cards[i].Tags = append([]string{}, board.Cards[i].Tags...)
		body.Cards[i].Comments = append([]Comment{}, board.Cards[i].Comments...)
		body.Cards[i].Checks = append([]Check{}, board.Cards[i].Checks...)
		body.Cards[i].Links = append([]Link{}, board.Cards[i].Links...)
		if body.Cards[i].Tags == nil {
			body.Cards[i].Tags = []string{}
		}
		if body.Cards[i].Comments == nil {
			body.Cards[i].Comments = []Comment{}
		}
		if body.Cards[i].Checks == nil {
			body.Cards[i].Checks = []Check{}
		}
		if body.Cards[i].Links == nil {
			body.Cards[i].Links = []Link{}
		}
	}
	return body
}

func cloneRec(board boardRec) boardRec {
	board.MemberIDs = append([]string(nil), board.MemberIDs...)
	board.Columns = append([]Column(nil), board.Columns...)
	board.Events = append([]Event(nil), board.Events...)
	cards := make([]Card, len(board.Cards))
	for i := range board.Cards {
		cards[i] = board.Cards[i]
		cards[i].Tags = append([]string(nil), board.Cards[i].Tags...)
		cards[i].Comments = append([]Comment(nil), board.Cards[i].Comments...)
		cards[i].Checks = append([]Check(nil), board.Cards[i].Checks...)
		cards[i].Links = append([]Link(nil), board.Cards[i].Links...)
	}
	board.Cards = cards
	return board
}

func (s *BoardStore) ensureDemoLocked() {
	for _, board := range s.boards {
		if board.ID == "demo" {
			return
		}
	}
	s.boards = append([]boardRec{seedDemo()}, s.boards...)
}

func (s *BoardStore) ensurePeopleLocked() {
	owner := s.demoUserLocked("owner@demo.local", "Аня")
	member := s.demoUserLocked("member@demo.local", "Илья")
	for _, board := range s.boards {
		if board.ID == "sprint" {
			return
		}
	}
	s.boards = append(s.boards, boardRec{
		ID: "sprint", Title: "Спринт", Owner: owner, MemberIDs: []string{member},
		Columns: templateColumns("dev"),
		Cards: []Card{{
			ID: "c" + newID(), Column: "", Pos: 0, Title: "Показать доску команды",
			Description: "Владелец Аня, участник Илья.", Assignee: "Илья", Due: "2026-10-30",
			Tags: []string{"команда"}, Comments: []Comment{}, Checks: []Check{}, Links: []Link{},
		}},
	})
	last := &s.boards[len(s.boards)-1]
	last.Cards[0].Column = last.Columns[0].ID
}

func (s *BoardStore) demoUserLocked(email, name string) string {
	for _, user := range s.users {
		if user.Email == email {
			return user.ID
		}
	}
	user := boardUser{ID: labelID(), Name: name, Email: email, Created: time.Now().Unix()}
	s.users = append(s.users, user)
	return user.ID
}

func (s *BoardStore) find(id string) *boardRec {
	for i := range s.boards {
		if s.boards[i].ID == id {
			return &s.boards[i]
		}
	}
	return nil
}

func (s *BoardStore) userName(id string) string {
	for _, user := range s.users {
		if user.ID == id {
			return user.Name
		}
	}
	return ""
}

func (s *BoardStore) publicLocked(id string) *PublicUser {
	if id == "" {
		return nil
	}
	for _, user := range s.users {
		if user.ID == id {
			pub := user.public()
			return &pub
		}
	}
	return nil
}

func (s *BoardStore) saveLocked() error {
	if s.path == "" {
		return nil
	}
	payload, err := json.Marshal(boardDisk{Users: s.users, Boards: s.boards})
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *BoardStore) note(board *boardRec, text string) {
	board.Events = append(board.Events, Event{At: time.Now().Format("15:04"), Text: text})
	if len(board.Events) > 30 {
		board.Events = board.Events[len(board.Events)-30:]
	}
}

func (user boardUser) public() PublicUser {
	return PublicUser{ID: user.ID, Name: user.Name, Email: user.Email, Role: "member"}
}

func canView(board *boardRec, actor Actor) bool {
	if board.Public || board.Owner == actor.ID {
		return true
	}
	return actor.ID != "" && contains(board.MemberIDs, actor.ID)
}

func canEdit(board *boardRec, actor Actor) bool {
	return canView(board, actor)
}

func templateColumns(name string) []Column {
	switch name {
	case "launch":
		return []Column{{ID: "l" + newID(), Title: "Идеи"}, {ID: "l" + newID(), Title: "Делаем"}, {ID: "l" + newID(), Title: "Проверка"}, {ID: "l" + newID(), Title: "Готово"}}
	case "empty":
		return []Column{{ID: "l" + newID(), Title: "Сделать"}}
	default:
		return []Column{{ID: "l" + newID(), Title: "Бэклог"}, {ID: "l" + newID(), Title: "В работе"}, {ID: "l" + newID(), Title: "Готово"}}
	}
}

func columnTitle(board *boardRec, id string) string {
	for _, column := range board.Columns {
		if column.ID == id {
			return column.Title
		}
	}
	return ""
}

func findCard(board *boardRec, id string) *Card {
	for i := range board.Cards {
		if board.Cards[i].ID == id {
			return &board.Cards[i]
		}
	}
	return nil
}

func nextPos(board *boardRec, column string) int {
	max := 0
	seen := false
	for _, card := range board.Cards {
		if card.Column == column && (!seen || card.Pos >= max) {
			max = card.Pos + 1
			seen = true
		}
	}
	return max
}

func splitTags(value string) []string {
	parts := strings.Split(value, ",")
	out := []string{}
	seen := map[string]bool{}
	for _, part := range parts {
		tag := clean(part, 24)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
		if len(out) == 4 {
			break
		}
	}
	return out
}

func safeURL(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://") {
		if len(value) <= 200 && !strings.ContainsAny(value, " \n") {
			return value, true
		}
	}
	if strings.HasPrefix(value, "/") && !strings.Contains(value, "//") && !strings.Contains(value, " ") && len(value) < 120 {
		return value, true
	}
	return "", false
}

func guestName(actor Actor) string {
	if actor.Name != "" {
		return actor.Name
	}
	return "Гость"
}

func contains(list []string, id string) bool {
	for _, item := range list {
		if item == id {
			return true
		}
	}
	return false
}

func ping(id string) []byte {
	payload, _ := json.Marshal(map[string]string{"type": "board", "id": id})
	return payload
}

func mustView(state BoardState, err error) BoardState {
	if err != nil {
		return BoardState{Board: BoardBody{Columns: []Column{}, Cards: []Card{}, Events: []Event{}, Members: []Member{}}, Boards: []BoardRef{}}
	}
	return state
}
