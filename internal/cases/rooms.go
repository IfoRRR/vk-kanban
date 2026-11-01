package cases

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Room struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Seats int    `json:"seats"`
	Floor string `json:"floor"`
}

type Booking struct {
	ID      string `json:"id"`
	Room    string `json:"room"`
	Date    string `json:"date"`
	Start   string `json:"start"`
	Minutes int    `json:"minutes"`
	Who     string `json:"who"`
	Title   string `json:"title"`
	Owner   string `json:"owner,omitempty"`
}

type PublicUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type Actor struct {
	ID   string
	Name string
	Role string
}

type RoomsState struct {
	Rooms    []Room      `json:"rooms"`
	Bookings []Booking   `json:"bookings"`
	User     *PublicUser `json:"user,omitempty"`
	Upcoming []Booking   `json:"upcoming"`
}

type WeekDay struct {
	Date    string `json:"date"`
	Label   string `json:"label"`
	Count   int    `json:"count"`
	Minutes int    `json:"minutes"`
}

type RoomUsage struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Meetings  int    `json:"meetings"`
	Minutes   int    `json:"minutes"`
	Occupancy int    `json:"occupancy"`
}

type Report struct {
	From  string      `json:"from"`
	To    string      `json:"to"`
	Days  int         `json:"days"`
	Rooms []RoomUsage `json:"rooms"`
}

type roomUser struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Role    string `json:"role"`
	Salt    string `json:"salt"`
	Pass    string `json:"pass"`
	Token   string `json:"token,omitempty"`
	Created int64  `json:"created"`
}

type roomsDisk struct {
	Rooms    []Room     `json:"rooms"`
	Bookings []Booking  `json:"bookings"`
	Users    []roomUser `json:"users"`
}

type RoomStore struct {
	mu       sync.Mutex
	path     string
	rooms    []Room
	bookings []Booking
	users    []roomUser
}

func NewRooms() *RoomStore {
	return &RoomStore{rooms: seedRooms(), bookings: seedBookings()}
}

func OpenRooms(path string) (*RoomStore, error) {
	store := NewRooms()
	if path == "" {
		return store, nil
	}
	store.path = path
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, store.saveLocked()
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return store, store.saveLocked()
	}
	var disk roomsDisk
	if err := json.Unmarshal(data, &disk); err != nil {
		return nil, err
	}
	if len(disk.Rooms) > 0 {
		store.rooms = disk.Rooms
	}
	if disk.Bookings != nil {
		store.bookings = disk.Bookings
	}
	store.users = disk.Users
	return store, nil
}

func seedRooms() []Room {
	return []Room{
		{ID: "alpha", Name: "Альфа", Seats: 8, Floor: "2 этаж"},
		{ID: "beta", Name: "Бета", Seats: 6, Floor: "2 этаж"},
		{ID: "gamma", Name: "Гамма", Seats: 4, Floor: "3 этаж"},
		{ID: "small", Name: "Малая", Seats: 2, Floor: "3 этаж"},
	}
}

func seedBookings() []Booking {
	return []Booking{
		{ID: "b1", Room: "alpha", Date: "today", Start: "10:00", Minutes: 60, Who: "Аня", Title: "План спринта"},
		{ID: "b2", Room: "small", Date: "today", Start: "11:00", Minutes: 30, Who: "Илья", Title: "Созвон"},
	}
}

func (s *RoomStore) Snapshot(date string, actor Actor) RoomsState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.viewLocked(date, actor)
}

func (s *RoomStore) Book(date, room, start, who, title string, minutes int, actor Actor) (RoomsState, error) {
	when, startMin, title, err := slotSpec(date, start, title, minutes)
	if err != nil {
		return RoomsState{}, err
	}
	who = fallback(clean(who, 24), "Гость")
	s.mu.Lock()
	defer s.mu.Unlock()
	if !knownRoom(s.rooms, room) {
		return RoomsState{}, errMissing
	}
	if conflicts(s.bookings, room, when, startMin, minutes, "") {
		return RoomsState{}, errBusy
	}
	if len(s.bookings) >= 500 {
		return RoomsState{}, Error("Слишком много броней, сбросьте демо")
	}
	item := Booking{
		ID: "b" + newID(), Room: room, Date: when, Start: formatClock(startMin),
		Minutes: minutes, Who: who, Title: title,
	}
	if actor.ID != "" {
		item.Owner = actor.ID
		item.Who = actor.Name
	}
	s.bookings = append(s.bookings, item)
	if err := s.saveLocked(); err != nil {
		return RoomsState{}, errStore
	}
	return s.viewLocked(when, actor), nil
}

func (s *RoomStore) Update(id string, actor Actor, date, room, start, title, view string, minutes int) (RoomsState, error) {
	when, startMin, title, err := slotSpec(date, start, title, minutes)
	if err != nil {
		return RoomsState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := bookingIndex(s.bookings, id)
	if idx < 0 {
		return RoomsState{}, errMissing
	}
	if !canTouch(s.bookings[idx], actor) {
		return RoomsState{}, errForbidden
	}
	if !knownRoom(s.rooms, room) {
		return RoomsState{}, errMissing
	}
	if conflicts(s.bookings, room, when, startMin, minutes, id) {
		return RoomsState{}, errBusy
	}
	item := s.bookings[idx]
	item.Room = room
	item.Date = when
	item.Start = formatClock(startMin)
	item.Minutes = minutes
	item.Title = title
	s.bookings[idx] = item
	if err := s.saveLocked(); err != nil {
		return RoomsState{}, errStore
	}
	if view == "" {
		view = when
	}
	return s.viewLocked(view, actor), nil
}

func (s *RoomStore) Cancel(id, date string, actor Actor) (RoomsState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := bookingIndex(s.bookings, id)
	if idx < 0 {
		return RoomsState{}, errMissing
	}
	if !canTouch(s.bookings[idx], actor) {
		return RoomsState{}, errForbidden
	}
	next := make([]Booking, 0, len(s.bookings)-1)
	for i, item := range s.bookings {
		if i != idx {
			next = append(next, item)
		}
	}
	s.bookings = next
	if err := s.saveLocked(); err != nil {
		return RoomsState{}, errStore
	}
	return s.viewLocked(date, actor), nil
}

func (s *RoomStore) Reset() RoomsState {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bookings = seedBookings()
	_ = s.saveLocked()
	return s.viewLocked("", Actor{})
}

func (s *RoomStore) Week(date string) ([]WeekDay, error) {
	anchor, ok := parseDate(date)
	if !ok {
		return nil, errDate
	}
	offset := (int(anchor.Weekday()) + 6) % 7
	monday := anchor.AddDate(0, 0, -offset)
	names := []string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}
	s.mu.Lock()
	defer s.mu.Unlock()
	days := make([]WeekDay, 0, 7)
	for i := 0; i < 7; i++ {
		day := monday.AddDate(0, 0, i)
		key := day.Format("2006-01-02")
		item := WeekDay{Date: key, Label: names[int(day.Weekday())] + " " + day.Format("02")}
		for _, booking := range s.onDay(key) {
			item.Count++
			item.Minutes += booking.Minutes
		}
		days = append(days, item)
	}
	return days, nil
}

func (s *RoomStore) Free(date, start string, minutes int) ([]Room, error) {
	when, startMin, _, err := slotSpec(date, start, "поиск", minutes)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Room{}
	for _, room := range s.rooms {
		if !conflicts(s.bookings, room.ID, when, startMin, minutes, "") {
			out = append(out, room)
		}
	}
	return out, nil
}

func (s *RoomStore) Report(from, to string) (Report, error) {
	days, err := eachDay(from, to)
	if err != nil {
		return Report{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	usage := map[string]*RoomUsage{}
	report := Report{From: from, To: to, Days: len(days), Rooms: make([]RoomUsage, 0, len(s.rooms))}
	for _, room := range s.rooms {
		usage[room.ID] = &RoomUsage{ID: room.ID, Name: room.Name}
	}
	for _, day := range days {
		for _, booking := range s.onDay(day) {
			row := usage[booking.Room]
			if row == nil {
				continue
			}
			row.Meetings++
			row.Minutes += booking.Minutes
		}
	}
	span := len(days) * 9 * 60
	for _, room := range s.rooms {
		row := usage[room.ID]
		if span > 0 {
			row.Occupancy = row.Minutes * 100 / span
		}
		report.Rooms = append(report.Rooms, *row)
	}
	return report, nil
}

func (s *RoomStore) Events(from, to string) ([]Booking, error) {
	days, err := eachDay(from, to)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Booking{}
	for _, day := range days {
		for _, booking := range s.onDay(day) {
			copy := booking
			copy.Date = day
			out = append(out, copy)
		}
	}
	return out, nil
}

func (s *RoomStore) Register(name, email, password, role string) (PublicUser, string, error) {
	label := clean(name, 24)
	if len([]rune(label)) < 2 {
		return PublicUser{}, "", errShort
	}
	mail := roomEmail(email)
	if mail == "" || len(password) < 6 || len(password) > 72 {
		return PublicUser{}, "", errAuth
	}
	if role != "employee" && role != "admin" {
		return PublicUser{}, "", errRole
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
	user := roomUser{
		ID: labelID(), Name: label, Email: mail, Role: role,
		Salt: salt, Pass: stretch(password, salt), Created: time.Now().Unix(),
	}
	user.Token = newSecret()
	s.users = append(s.users, user)
	if err := s.saveLocked(); err != nil {
		return PublicUser{}, "", errStore
	}
	return user.public(), user.Token, nil
}

func (s *RoomStore) Login(email, password string) (PublicUser, string, error) {
	mail := roomEmail(email)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, user := range s.users {
		if user.Email != mail {
			continue
		}
		if subtlePass(user.Pass, stretch(password, user.Salt)) {
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

func (s *RoomStore) Demo(role string) (PublicUser, string, error) {
	if role != "employee" && role != "admin" {
		return PublicUser{}, "", errRole
	}
	email := "employee@demo.local"
	name := "Аня"
	if role == "admin" {
		email = "admin@demo.local"
		name = "Илья"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, user := range s.users {
		if user.Email == email {
			s.users[i].Token = newSecret()
			if err := s.saveLocked(); err != nil {
				return PublicUser{}, "", errStore
			}
			return s.users[i].public(), s.users[i].Token, nil
		}
	}
	user := roomUser{
		ID: labelID(), Name: name, Email: email, Role: role, Created: time.Now().Unix(),
	}
	user.Token = newSecret()
	s.users = append(s.users, user)
	if err := s.saveLocked(); err != nil {
		return PublicUser{}, "", errStore
	}
	return user.public(), user.Token, nil
}

func (s *RoomStore) Logout(token string) {
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

func (s *RoomStore) UserByToken(token string) (PublicUser, error) {
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

func (s *RoomStore) viewLocked(date string, actor Actor) RoomsState {
	return RoomsState{
		Rooms:    append([]Room(nil), s.rooms...),
		Bookings: s.onDay(date),
		User:     s.publicLocked(actor.ID),
		Upcoming: s.upcomingLocked(actor.ID),
	}
}

func (s *RoomStore) onDay(date string) []Booking {
	list := []Booking{}
	for _, item := range s.bookings {
		if item.Date == "today" || (date != "" && item.Date == date) {
			list = append(list, item)
		}
	}
	return list
}

func (s *RoomStore) upcomingLocked(userID string) []Booking {
	if userID == "" {
		return []Booking{}
	}
	today := time.Now().Format("2006-01-02")
	list := []Booking{}
	for _, item := range s.bookings {
		if item.Owner != userID {
			continue
		}
		if item.Date == "today" || item.Date >= today {
			list = append(list, item)
		}
	}
	if len(list) > 8 {
		list = list[:8]
	}
	return list
}

func (s *RoomStore) publicLocked(id string) *PublicUser {
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

func (s *RoomStore) saveLocked() error {
	if s.path == "" {
		return nil
	}
	payload, err := json.MarshalIndent(roomsDisk{Rooms: s.rooms, Bookings: s.bookings, Users: s.users}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (user roomUser) public() PublicUser {
	return PublicUser{ID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role}
}

func slotSpec(date, start, title string, minutes int) (string, int, string, error) {
	date = clean(date, 10)
	if date != "today" && !validDate(date) {
		return "", 0, "", errDate
	}
	if minutes != 30 && minutes != 60 && minutes != 90 {
		return "", 0, "", errSlot
	}
	startMin, ok := parseClock(start)
	if !ok || startMin%30 != 0 || startMin < 9*60 || startMin+minutes > 18*60 {
		return "", 0, "", errSlot
	}
	return date, startMin, fallback(clean(title, 60), "Встреча"), nil
}

func conflicts(bookings []Booking, room, date string, start, minutes int, ignoreID string) bool {
	for _, item := range bookings {
		if item.ID == ignoreID || item.Room != room {
			continue
		}
		if date != "today" && item.Date != "today" && item.Date != date {
			continue
		}
		if overlaps(start, minutes, item) {
			return true
		}
	}
	return false
}

func canTouch(item Booking, actor Actor) bool {
	if item.Owner == "" || actor.Role == "admin" {
		return true
	}
	return actor.ID != "" && actor.ID == item.Owner
}

func bookingIndex(bookings []Booking, id string) int {
	for i, item := range bookings {
		if item.ID == id {
			return i
		}
	}
	return -1
}

func knownRoom(rooms []Room, id string) bool {
	for _, room := range rooms {
		if room.ID == id {
			return true
		}
	}
	return false
}

func overlaps(start, minutes int, item Booking) bool {
	other, ok := parseClock(item.Start)
	if !ok {
		return false
	}
	return start < other+item.Minutes && other < start+minutes
}

func parseClock(value string) (int, bool) {
	if len(value) != 5 || value[2] != ':' {
		return 0, false
	}
	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	min := int(value[3]-'0')*10 + int(value[4]-'0')
	if hour > 23 || min > 59 || value[0] < '0' || value[1] < '0' || value[3] < '0' || value[4] < '0' {
		return 0, false
	}
	return hour*60 + min, true
}

func formatClock(mins int) string {
	hour := mins / 60
	min := mins % 60
	return string([]byte{byte('0' + hour/10), byte('0' + hour%10), ':', byte('0' + min/10), byte('0' + min%10)})
}

func validDate(value string) bool {
	_, ok := parseDate(value)
	return ok
}

func parseDate(value string) (time.Time, bool) {
	if len(value) != 10 {
		return time.Time{}, false
	}
	parsed, err := time.Parse("2006-01-02", value)
	return parsed, err == nil
}

func eachDay(from, to string) ([]string, error) {
	start, okStart := parseDate(from)
	end, okEnd := parseDate(to)
	if !okStart || !okEnd || end.Before(start) {
		return nil, errDate
	}
	days := []string{}
	for !start.After(end) {
		days = append(days, start.Format("2006-01-02"))
		if len(days) > 62 {
			return nil, errRange
		}
		start = start.AddDate(0, 0, 1)
	}
	return days, nil
}

func roomEmail(value string) string {
	value = strings.ToLower(clean(value, 80))
	if !strings.Contains(value, "@") || strings.Contains(value, " ") {
		return ""
	}
	parts := strings.Split(value, "@")
	if len(parts) != 2 || parts[0] == "" || !strings.Contains(parts[1], ".") {
		return ""
	}
	return value
}

func stretch(password, salt string) string {
	sum := sha256.Sum256([]byte(salt + "\n" + password))
	for i := 0; i < 20000; i++ {
		sum = sha256.Sum256(sum[:])
	}
	return hex.EncodeToString(sum[:])
}

func subtlePass(stored, got string) bool {
	if len(stored) != len(got) || stored == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(got)) == 1
}

func sameSecret(stored, got string) bool {
	if len(stored) != len(got) || stored == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(got)) == 1
}

func newSecret() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func labelID() string {
	return newSecret()[:12]
}
