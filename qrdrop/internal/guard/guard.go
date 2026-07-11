// Package guard は LanGuard セキュリティミドルウェアを提供する(DES-004)。
// 全要求に対して (1) 接続元がプライベート IP であること、
// (2) /admin はループバックのみ、(3) /s/<token>/... はトークン有効性、を検査し、
// 違反は 403 で遮断する(REQ-004, NFR-002)。
// あわせてパストラバーサル防止のファイル名サニタイズを提供する。
package guard

import (
	"net"
	"net/http"
	"strings"

	"qrdrop/internal/session"
)

// Logger は guard が必要とするログ機能(logging.Logger が満たす)。
type Logger interface {
	Deny(remote, path, reason string)
}

// IsPrivate は ip が RFC1918 プライベート範囲・リンクローカル・ループバックの
// いずれかに含まれるかを返す(要件定義書 §1「プライベート IP」)。
func IsPrivate(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}

// remoteIP は http.Request.RemoteAddr("host:port")から IP を取り出す。
func remoteIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr // ポートなしの形式にも耐える
	}
	// IPv6 ゾーン識別子(fe80::1%eth0)を除去
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	return net.ParseIP(host)
}

// ErrorPageFunc は 403 応答の本文を書く関数(web パッケージが供給する)。
type ErrorPageFunc func(w http.ResponseWriter, r *http.Request, status int, message string)

// Guard は LanGuard ミドルウェア本体。
type Guard struct {
	Sessions  *session.Manager
	Log       Logger
	ErrorPage ErrorPageFunc // nil なら http.Error
}

func (g *Guard) forbid(w http.ResponseWriter, r *http.Request, reason, message string) {
	g.Log.Deny(r.RemoteAddr, r.URL.Path, reason)
	if g.ErrorPage != nil {
		g.ErrorPage(w, r, http.StatusForbidden, message)
		return
	}
	http.Error(w, message, http.StatusForbidden)
}

// Middleware は next を LanGuard 検査で包む。
func (g *Guard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := remoteIP(r.RemoteAddr)

		// 第1層: プライベート IP 以外はすべて拒否(REQ-004)
		if !IsPrivate(ip) {
			g.forbid(w, r, "public-ip", "アクセスできません")
			return
		}

		// 第2層: /admin はループバック(PC 自身)のみ(§7-2)
		if r.URL.Path == "/admin" || strings.HasPrefix(r.URL.Path, "/admin/") {
			if !ip.IsLoopback() {
				g.forbid(w, r, "admin-not-loopback", "アクセスできません")
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// 第3層: /s/<token>... はトークン有効性を検査(REQ-005)
		if strings.HasPrefix(r.URL.Path, "/s/") {
			token := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/s/"), "/", 2)[0]
			if err := g.Sessions.Validate(token); err != nil {
				g.forbid(w, r, "invalid-token: "+err.Error(),
					"セッションが無効です。PC の管理ページに表示されている QR コードを読み取り直してください。")
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// 第4層: 上記以外のパス(`/` 直叩き等)は拒否(REQ-004 受け入れ基準)
		g.forbid(w, r, "no-token-path", "アクセスできません")
	})
}

// forbiddenChars は Windows のファイル名で使えない文字と ..(トラバーサル)対策。
const forbiddenChars = `<>:"/\|?*`

// SanitizeFilename はアップロードされたファイル名を保存に安全な形へ変換する。
// パス区切り(/, \)を除去してベース名のみとし、禁止文字・制御文字を除去する。
// 空になった場合は "file" を返す(NFR-002 パストラバーサル防止)。
func SanitizeFilename(name string) string {
	// / と \ の両方をパス区切りとして最後の要素のみ採用
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(forbiddenChars, r) {
			continue
		}
		b.WriteRune(r)
	}
	name = strings.Trim(b.String(), " .") // 末尾ドット・空白は Windows で不可、"." ".." も無害化
	if name == "" {
		return "file"
	}
	return name
}
