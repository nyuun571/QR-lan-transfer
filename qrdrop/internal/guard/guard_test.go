package guard

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"qrdrop/internal/session"
)

type nopLog struct{}

func (nopLog) Deny(remote, path, reason string) {}

func TestIsPrivate(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"10.0.0.5", true},
		{"172.16.0.1", true},
		{"172.31.255.254", true},
		{"192.168.1.20", true},
		{"169.254.10.10", true}, // リンクローカル
		{"127.0.0.1", true},     // ループバック
		{"::1", true},
		{"fe80::1", true},
		{"8.8.8.8", false},     // パブリック
		{"172.32.0.1", false},  // RFC1918 範囲外
		{"203.0.113.7", false}, // TEST-NET
		{"2001:db8::1", false}, // グローバル IPv6
	}
	for _, tt := range tests {
		if got := IsPrivate(net.ParseIP(tt.ip)); got != tt.want {
			t.Errorf("IsPrivate(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
	if IsPrivate(nil) {
		t.Error("IsPrivate(nil) = true, want false")
	}
}

func newGuard(t *testing.T) (*Guard, *session.Manager) {
	t.Helper()
	sm := session.NewManager(30 * time.Minute)
	return &Guard{Sessions: sm, Log: nopLog{}}, sm
}

// serve は remoteAddr を偽装して1リクエストを流し、ステータスを返す。
func serve(g *Guard, remoteAddr, path string) int {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	g.Middleware(ok).ServeHTTP(rec, req)
	return rec.Code
}

func TestMiddlewarePublicIPBlocked(t *testing.T) {
	g, sm := newGuard(t)
	if got := serve(g, "203.0.113.7:40000", "/s/"+sm.Token()); got != 403 {
		t.Errorf("パブリック IP: status = %d, want 403(正しいトークンでも遮断)", got)
	}
}

func TestMiddlewareAdminLoopbackOnly(t *testing.T) {
	g, _ := newGuard(t)
	if got := serve(g, "127.0.0.1:50000", "/admin"); got != 200 {
		t.Errorf("ループバックから /admin: status = %d, want 200", got)
	}
	if got := serve(g, "127.0.0.1:50000", "/admin/qr.png"); got != 200 {
		t.Errorf("ループバックから /admin/qr.png: status = %d, want 200", got)
	}
	if got := serve(g, "192.168.1.20:50000", "/admin"); got != 403 {
		t.Errorf("LAN 内他端末から /admin: status = %d, want 403", got)
	}
}

func TestMiddlewareTokenPaths(t *testing.T) {
	g, sm := newGuard(t)
	tok := sm.Token()
	if got := serve(g, "192.168.1.20:50000", "/s/"+tok); got != 200 {
		t.Errorf("有効トークン: status = %d, want 200", got)
	}
	if got := serve(g, "192.168.1.20:50000", "/s/"+tok+"/upload"); got != 200 {
		t.Errorf("有効トークン(サブパス): status = %d, want 200", got)
	}
	if got := serve(g, "192.168.1.20:50000", "/s/wrongtoken/upload"); got != 403 {
		t.Errorf("無効トークン: status = %d, want 403", got)
	}
	sm.Revoke()
	if got := serve(g, "192.168.1.20:50000", "/s/"+tok); got != 403 {
		t.Errorf("失効トークン: status = %d, want 403", got)
	}
}

func TestMiddlewareRootBlocked(t *testing.T) {
	g, _ := newGuard(t)
	if got := serve(g, "192.168.1.20:50000", "/"); got != 403 {
		t.Errorf("トークンなし `/` 直叩き: status = %d, want 403(REQ-004)", got)
	}
	if got := serve(g, "192.168.1.20:50000", "/s/"); got != 403 {
		t.Errorf("空トークン: status = %d, want 403", got)
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct{ in, want string }{
		{"photo.jpg", "photo.jpg"},
		{"../../etc/passwd", "passwd"},
		{`..\..\windows\system32\evil.exe`, "evil.exe"},
		{`C:\Users\x\doc.txt`, "doc.txt"},
		{"a/b/c.png", "c.png"},
		{`con<>:"|?*fig.txt`, "config.txt"},
		{"..", "file"},
		{".", "file"},
		{"", "file"},
		{"  spaced  .txt", "spaced  .txt"},
		{"名前 (1).jpg", "名前 (1).jpg"}, // 日本語・括弧は保持
		{"bad\x00\x1fname", "badname"},
		{"trailing...", "trailing"},
	}
	for _, tt := range tests {
		if got := SanitizeFilename(tt.in); got != tt.want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
