package cases

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Sample struct {
	At    string `json:"at"`
	OK    bool   `json:"ok"`
	MS    int    `json:"ms"`
	Error string `json:"error,omitempty"`
	Code  int    `json:"code,omitempty"`
}

type Incident struct {
	ID     string `json:"id"`
	Opened string `json:"opened"`
	Closed string `json:"closed,omitempty"`
	Error  string `json:"error"`
}

type ServiceStatus struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	URL          string     `json:"url"`
	Kind         string     `json:"kind"`
	Expect       string     `json:"expect,omitempty"`
	Seed         bool       `json:"seed"`
	OK           bool       `json:"ok"`
	MS           int        `json:"ms"`
	Checked      string     `json:"checked"`
	Error        string     `json:"error,omitempty"`
	Availability int        `json:"availability"`
	AvgMS        int        `json:"avgMs"`
	Checks       int        `json:"checks"`
	History      []Sample   `json:"history"`
	Incidents    []Incident `json:"incidents"`
}

type watchTarget struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	URL       string     `json:"url"`
	Show      string     `json:"show"`
	Kind      string     `json:"kind"`
	Expect    string     `json:"expect"`
	Seed      bool       `json:"seed"`
	Enabled   bool       `json:"enabled"`
	History   []Sample   `json:"history"`
	Incidents []Incident `json:"incidents"`
}

type Monitor struct {
	mu      sync.Mutex
	path    string
	targets []watchTarget
	client  *http.Client
	now     func() time.Time
	running atomic.Bool
}

func NewMonitor() *Monitor {
	return &Monitor{
		client: newWatchClient(),
		now:    time.Now,
	}
}

func OpenMonitor(path string) (*Monitor, error) {
	monitor := NewMonitor()
	monitor.path = path
	if path == "" {
		monitor.targets = seedTargets()
		return monitor, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil && len(strings.TrimSpace(string(data))) > 0 {
		var disk struct {
			Targets []watchTarget `json:"targets"`
		}
		if err := json.Unmarshal(data, &disk); err != nil {
			return nil, err
		}
		monitor.targets = disk.Targets
	}
	monitor.ensureSeedsLocked()
	return monitor, monitor.saveLocked()
}

func seedTargets() []watchTarget {
	return []watchTarget{
		{ID: "site", Name: "Сайт", URL: "http://127.0.0.1:8081/", Show: "/", Kind: "http", Seed: true, Enabled: true},
		{ID: "chat", Name: "Чат", URL: "http://127.0.0.1:8081/chat/ping", Show: "/chat/ping", Kind: "body", Expect: "pong", Seed: true, Enabled: true},
		{ID: "quiz", Name: "Квиз", URL: "http://127.0.0.1:8081/quiz/ping", Show: "/quiz/ping", Kind: "body", Expect: "pong", Seed: true, Enabled: true},
		{ID: "feed", Name: "Лента", URL: "http://127.0.0.1:8081/api/feed/health", Show: "/api/feed/health", Kind: "http", Seed: true, Enabled: true},
		{ID: "music", Name: "Музыка", URL: "http://127.0.0.1:8081/api/vk-music/health", Show: "/api/vk-music/health", Kind: "http", Seed: true, Enabled: true},
	}
}

func (m *Monitor) Snapshot(query, status string) []ServiceStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	needle := strings.ToLower(strings.TrimSpace(query))
	out := []ServiceStatus{}
	for _, target := range m.targets {
		item := m.view(target)
		if needle != "" && !strings.Contains(strings.ToLower(item.Name+" "+item.URL), needle) {
			continue
		}
		switch status {
		case "up":
			if item.Checked == "" || !item.OK {
				continue
			}
		case "down":
			if item.Checked == "" || item.OK {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func (m *Monitor) One(id string) (ServiceStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	target := m.find(id)
	if target == nil {
		return ServiceStatus{}, errMissing
	}
	return m.view(*target), nil
}

func (m *Monitor) Add(name, raw, kind, expect string) ([]ServiceStatus, error) {
	name = clean(name, 40)
	if len([]rune(name)) < 2 {
		return nil, errShort
	}
	kind, expect, err := checkKind(kind, expect)
	if err != nil {
		return nil, err
	}
	fetch, show, err := normalizeTarget(raw)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	custom := 0
	for _, target := range m.targets {
		if !target.Seed {
			custom++
		}
		if target.URL == fetch {
			return nil, Error("Такой адрес уже есть")
		}
	}
	if custom >= 8 {
		return nil, Error("Список проверок заполнен")
	}
	m.targets = append(m.targets, watchTarget{
		ID: "w" + newID(), Name: name, URL: fetch, Show: show, Kind: kind, Expect: expect, Enabled: true,
		History: []Sample{}, Incidents: []Incident{},
	})
	if err := m.saveLocked(); err != nil {
		return nil, errStore
	}
	return m.viewsLocked(), nil
}

func (m *Monitor) Update(id, name, raw, kind, expect string) ([]ServiceStatus, error) {
	name = clean(name, 40)
	if len([]rune(name)) < 2 {
		return nil, errShort
	}
	kind, expect, err := checkKind(kind, expect)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	target := m.find(id)
	if target == nil {
		return nil, errMissing
	}
	target.Name = name
	target.Kind = kind
	target.Expect = expect
	if !target.Seed && raw != "" {
		fetch, show, err := normalizeTarget(raw)
		if err != nil {
			return nil, err
		}
		target.URL = fetch
		target.Show = show
	}
	if err := m.saveLocked(); err != nil {
		return nil, errStore
	}
	return m.viewsLocked(), nil
}

func (m *Monitor) Delete(id string) ([]ServiceStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.targets[:0]
	found := false
	for _, target := range m.targets {
		if target.ID == id {
			if target.Seed {
				return nil, Error("Эту проверку портфолио нельзя снять")
			}
			found = true
			continue
		}
		next = append(next, target)
	}
	if !found {
		return nil, errMissing
	}
	m.targets = next
	if err := m.saveLocked(); err != nil {
		return nil, errStore
	}
	return m.viewsLocked(), nil
}

func (m *Monitor) Reset() []ServiceStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := map[string]watchTarget{}
	for _, target := range m.targets {
		if target.Seed {
			kept[target.ID] = target
		}
	}
	fresh := seedTargets()
	for i := range fresh {
		if old, ok := kept[fresh[i].ID]; ok {
			fresh[i].History = old.History
			fresh[i].Incidents = old.Incidents
		}
	}
	m.targets = fresh
	_ = m.saveLocked()
	return m.viewsLocked()
}

func (m *Monitor) CheckAll() []ServiceStatus {
	if !m.running.CompareAndSwap(false, true) {
		return m.Snapshot("", "")
	}
	defer m.running.Store(false)
	m.mu.Lock()
	targets := append([]watchTarget(nil), m.targets...)
	m.mu.Unlock()
	sem := make(chan struct{}, 3)
	var group sync.WaitGroup
	for _, target := range targets {
		if !target.Enabled {
			continue
		}
		group.Add(1)
		go func(item watchTarget) {
			defer group.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			m.record(item.ID, m.check(item))
		}(target)
	}
	group.Wait()
	m.mu.Lock()
	_ = m.saveLocked()
	m.mu.Unlock()
	return m.Snapshot("", "")
}

func (m *Monitor) check(target watchTarget) Sample {
	start := m.now()
	sample := Sample{At: start.Format(time.RFC3339)}
	request, err := http.NewRequest(http.MethodGet, target.URL, nil)
	if err != nil {
		sample.Error = "адрес не проверяем"
		return sample
	}
	request.Header.Set("User-Agent", "portfolio-monitor")
	response, err := m.client.Do(request)
	sample.MS = int(m.now().Sub(start).Milliseconds())
	if err != nil {
		sample.Error = "нет ответа"
		return sample
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
	sample.Code = response.StatusCode
	if response.StatusCode >= 400 {
		sample.Error = response.Status
		return sample
	}
	if target.Kind == "body" && !strings.Contains(string(body), target.Expect) {
		sample.Error = "нет ожидаемого ответа"
		return sample
	}
	sample.OK = true
	return sample
}

func (m *Monitor) record(id string, sample Sample) {
	m.mu.Lock()
	defer m.mu.Unlock()
	target := m.find(id)
	if target == nil {
		return
	}
	had := len(target.History) > 0
	wasOK := had && target.History[len(target.History)-1].OK
	target.History = append(target.History, sample)
	if len(target.History) > 48 {
		target.History = target.History[len(target.History)-48:]
	}
	if !sample.OK && (!had || wasOK) {
		target.Incidents = append(target.Incidents, Incident{ID: "i" + newID(), Opened: sample.At, Error: sample.Error})
		if len(target.Incidents) > 20 {
			target.Incidents = target.Incidents[len(target.Incidents)-20:]
		}
	}
	if sample.OK && had && !wasOK {
		for i := len(target.Incidents) - 1; i >= 0; i-- {
			if target.Incidents[i].Closed == "" {
				target.Incidents[i].Closed = sample.At
				break
			}
		}
	}
}

func (m *Monitor) view(target watchTarget) ServiceStatus {
	item := ServiceStatus{
		ID: target.ID, Name: target.Name, URL: target.Show, Kind: target.Kind, Expect: target.Expect,
		Seed: target.Seed, History: []Sample{}, Incidents: append([]Incident(nil), target.Incidents...),
	}
	for i := range item.Incidents {
		item.Incidents[i].Opened = shortWhen(item.Incidents[i].Opened)
		item.Incidents[i].Closed = shortWhen(item.Incidents[i].Closed)
	}
	okCount := 0
	sum := 0
	for _, sample := range target.History {
		view := sample
		if parsed, err := time.Parse(time.RFC3339, sample.At); err == nil {
			view.At = parsed.Local().Format("15:04:05")
		}
		item.History = append(item.History, view)
		if sample.OK {
			okCount++
			sum += sample.MS
		}
	}
	item.Checks = len(target.History)
	if item.Checks > 0 {
		last := target.History[len(target.History)-1]
		item.OK = last.OK
		item.MS = last.MS
		item.Error = last.Error
		item.Checked = item.History[len(item.History)-1].At
		item.Availability = okCount * 100 / item.Checks
	}
	if okCount > 0 {
		item.AvgMS = sum / okCount
	}
	if item.Incidents == nil {
		item.Incidents = []Incident{}
	}
	return item
}

func shortWhen(value string) string {
	if value == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return parsed.Local().Format("02.01 15:04")
}

func (m *Monitor) viewsLocked() []ServiceStatus {
	out := make([]ServiceStatus, 0, len(m.targets))
	for _, target := range m.targets {
		out = append(out, m.view(target))
	}
	return out
}

func (m *Monitor) find(id string) *watchTarget {
	for i := range m.targets {
		if m.targets[i].ID == id {
			return &m.targets[i]
		}
	}
	return nil
}

func (m *Monitor) ensureSeedsLocked() {
	have := map[string]bool{}
	for _, target := range m.targets {
		have[target.ID] = true
	}
	for _, seed := range seedTargets() {
		if !have[seed.ID] {
			m.targets = append(m.targets, seed)
		}
	}
}

func (m *Monitor) saveLocked() error {
	if m.path == "" {
		return nil
	}
	payload, err := json.Marshal(struct {
		Targets []watchTarget `json:"targets"`
	}{Targets: m.targets})
	if err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}

func checkKind(kind, expect string) (string, string, error) {
	expect = clean(expect, 40)
	if kind == "" || kind == "http" {
		return "http", "", nil
	}
	if kind == "body" && expect != "" {
		return "body", expect, nil
	}
	return "", "", Error("Для проверки тела укажите ожидаемый фрагмент")
}

func normalizeTarget(raw string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", "", errURL
	}
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	if portfolioHost(host) {
		if parsed.Scheme != "http" || port != "8081" {
			return "", "", errURL
		}
		path := parsed.EscapedPath()
		if path == "" {
			path = "/"
		}
		if parsed.RawQuery != "" {
			path += "?" + parsed.RawQuery
		}
		return "http://127.0.0.1:8081" + path, path, nil
	}
	if !webPort(port) {
		return "", "", errURL
	}
	if ip := net.ParseIP(host); ip != nil {
		if !publicIP(ip) {
			return "", "", errURL
		}
		return parsed.String(), parsed.String(), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := publicLookup(ctx, host); err != nil {
		return "", "", errURL
	}
	return parsed.String(), parsed.String(), nil
}

func portfolioHost(host string) bool {
	switch host {
	case "127.0.0.1", "localhost", "::1", "132.243.194.181":
		return true
	default:
		return false
	}
}

func webPort(port string) bool {
	switch port {
	case "80", "443", "3000", "5000", "8000", "8080", "8081", "8443":
		return true
	default:
		return false
	}
}

func publicLookup(ctx context.Context, host string) (string, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addrs) == 0 {
		return "", errURL
	}
	for _, addr := range addrs {
		if !publicIP(addr.IP) {
			return "", errURL
		}
	}
	return addrs[0].IP.String(), nil
}

func publicIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 0 || (v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127) {
			return false
		}
	}
	return true
}

func newWatchClient() *http.Client {
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			if portfolioHost(host) {
				if port != "8081" {
					return nil, errURL
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
			}
			if !webPort(port) {
				return nil, errURL
			}
			ip := net.ParseIP(host)
			if ip == nil {
				resolved, err := publicLookup(ctx, host)
				if err != nil {
					return nil, err
				}
				ip = net.ParseIP(resolved)
			}
			if !publicIP(ip) {
				return nil, errURL
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		},
		TLSHandshakeTimeout:   2 * time.Second,
		ResponseHeaderTimeout: 2 * time.Second,
		DisableKeepAlives:     true,
		MaxIdleConns:          2,
	}
	return &http.Client{
		Timeout:       3 * time.Second,
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
