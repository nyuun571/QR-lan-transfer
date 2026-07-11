package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"qrdrop/internal/logging"
	"qrdrop/internal/session"
	"qrdrop/internal/transfer"
)

func newTestServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	lg, err := logging.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lg.Close() })
	sm := session.NewManager(30 * time.Minute)
	th := &transfer.Handler{Store: transfer.NewStore(), RecvDir: t.TempDir(), Log: lg}
	s := NewServer(sm, th, lg, 8931, []string{"192.168.1.10", "10.0.0.5"})
	return s, s.Handler()
}

func get(h http.Handler, remote, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAdminPageServed(t *testing.T) {
	_, h := newTestServer(t)
	rec := get(h, "127.0.0.1:50000", "/admin")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "QRDrop 管理ページ") {
		t.Errorf("status = %d, 管理ページが返っていない", rec.Code)
	}
}

func TestQRPNG(t *testing.T) {
	_, h := newTestServer(t)
	rec := get(h, "127.0.0.1:50000", "/admin/qr.png")
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if !bytes.HasPrefix(rec.Body.Bytes(), []byte("\x89PNG")) {
		t.Error("PNG マジックバイトで始まっていない")
	}
}

func TestTransferPageServed(t *testing.T) {
	s, h := newTestServer(t)
	rec := get(h, "192.168.1.20:40000", "/s/"+s.Sessions.Token())
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "QRDrop 転送ページ") {
		t.Errorf("status = %d, 転送ページが返っていない", rec.Code)
	}
}

func TestAdminInfo(t *testing.T) {
	s, h := newTestServer(t)
	rec := get(h, "127.0.0.1:50000", "/admin/info")
	var info struct {
		IPs        []string `json:"ips"`
		SelectedIP string   `json:"selectedIp"`
		URL        string   `json:"url"`
		ExpiresAt  string   `json:"expiresAt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("JSON: %v (body=%s)", err, rec.Body)
	}
	want := "http://192.168.1.10:8931/s/" + s.Sessions.Token()
	if info.URL != want {
		t.Errorf("url = %q, want %q", info.URL, want)
	}
	if len(info.IPs) != 2 || info.SelectedIP != "192.168.1.10" {
		t.Errorf("ips = %v, selected = %s", info.IPs, info.SelectedIP)
	}
}

func TestRevokeInvalidatesOldURL(t *testing.T) {
	s, h := newTestServer(t)
	oldTok := s.Sessions.Token()

	req := httptest.NewRequest("POST", "/admin/session/revoke", nil)
	req.RemoteAddr = "127.0.0.1:50000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("revoke status = %d", rec.Code)
	}

	rec = get(h, "192.168.1.20:40000", "/s/"+oldTok)
	if rec.Code != 403 {
		t.Errorf("旧トークンの status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "読み取り直して") {
		t.Error("失効時の案内文(再読取)が表示されていない")
	}

	rec = get(h, "192.168.1.20:40000", "/s/"+s.Sessions.Token())
	if rec.Code != 200 {
		t.Errorf("新トークンの status = %d, want 200", rec.Code)
	}
}

func TestSelectInterface(t *testing.T) {
	s, h := newTestServer(t)
	post := func(ip string) int {
		req := httptest.NewRequest("POST", "/admin/interface",
			strings.NewReader(url.Values{"ip": {ip}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "127.0.0.1:50000"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := post("10.0.0.5"); code != 200 {
		t.Errorf("有効 IP の status = %d", code)
	}
	if got := s.SelectedIP(); got != "10.0.0.5" {
		t.Errorf("selectedIP = %s, want 10.0.0.5", got)
	}
	if code := post("8.8.8.8"); code != 400 {
		t.Errorf("不明 IP の status = %d, want 400", code)
	}
}

func TestSSEEvents(t *testing.T) {
	s, h := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest("GET", "/s/"+s.Sessions.Token()+"/events", nil).WithContext(ctx)
	req.RemoteAddr = "192.168.1.20:40000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req) // ctx タイムアウトまでブロック
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %s", ct)
	}
	if !strings.Contains(rec.Body.String(), "data: update") {
		t.Errorf("SSE 初期イベントがない: %q", rec.Body)
	}
}
