// Package web は管理ページ・転送ページの配信とルーティングを担う(DES-005, DES-006)。
// 静的アセットは go:embed でバイナリに同梱し、外部配信に依存しない。
package web

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sync"
	"time"

	"qrdrop/internal/guard"
	"qrdrop/internal/logging"
	"qrdrop/internal/session"
	"qrdrop/internal/transfer"
)

//go:embed static/admin.html
var adminHTML []byte

//go:embed static/transfer.html
var transferHTML []byte

//go:embed static/error.html
var errorHTMLSrc string

var errorTmpl = template.Must(template.New("error").Parse(errorHTMLSrc))

// Server はルーティングとセッション URL の組み立てを担う。
type Server struct {
	Sessions *session.Manager
	Transfer *transfer.Handler
	Log      *logging.Logger
	Port     int
	IPs      []string // 検出済みのプライベート IPv4(先頭が既定)

	mu         sync.Mutex
	selectedIP string
}

// NewServer は Server を初期化する(IPs の先頭を既定の LAN IP とする)。
func NewServer(sm *session.Manager, th *transfer.Handler, lg *logging.Logger, port int, ips []string) *Server {
	return &Server{Sessions: sm, Transfer: th, Log: lg, Port: port, IPs: ips, selectedIP: ips[0]}
}

// SelectedIP は現在選択中の LAN IP を返す。
func (s *Server) SelectedIP() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.selectedIP
}

// SessionURL はスマホが読み取る転送ページ URL を返す(REQ-001)。
func (s *Server) SessionURL() string {
	return fmt.Sprintf("http://%s:%d/s/%s", s.SelectedIP(), s.Port, s.Sessions.Token())
}

// ErrorPage は guard の 403 応答などに使う HTML エラーページを書く。
func (s *Server) ErrorPage(w http.ResponseWriter, r *http.Request, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	errorTmpl.Execute(w, map[string]any{"Status": status, "Message": message})
}

// Handler は LanGuard で包んだ全ルートのハンドラを返す。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 管理ページ(ループバック限定は guard が保証)
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(adminHTML)
	})
	mux.HandleFunc("GET /admin/qr.png", s.qrPNG)
	mux.HandleFunc("GET /admin/info", s.adminInfo)
	mux.HandleFunc("POST /admin/interface", s.selectInterface)
	mux.HandleFunc("POST /admin/share", s.Transfer.AdminShare)
	mux.HandleFunc("POST /admin/session/revoke", func(w http.ResponseWriter, r *http.Request) {
		s.Sessions.Revoke()
		s.Log.Info("セッションを手動失効し再発行しました")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /admin/items", s.Transfer.AdminItems)
	mux.HandleFunc("GET /admin/events", s.Transfer.Events)

	// 転送ページ(トークン有効性は guard が保証)
	mux.HandleFunc("GET /s/{token}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(transferHTML)
	})
	mux.HandleFunc("POST /s/{token}/upload", s.Transfer.Upload)
	mux.HandleFunc("GET /s/{token}/items", s.Transfer.Items)
	mux.HandleFunc("GET /s/{token}/files/{id}", s.Transfer.Download)
	mux.HandleFunc("POST /s/{token}/text", s.Transfer.TextFromPhone)
	mux.HandleFunc("GET /s/{token}/events", s.Transfer.Events)

	g := &guard.Guard{Sessions: s.Sessions, Log: s.Log, ErrorPage: s.ErrorPage}
	return g.Middleware(mux)
}

// adminInfo は管理ページ用の状態 JSON(GET /admin/info)。
func (s *Server) adminInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]any{
		"ips":        s.IPs,
		"selectedIp": s.SelectedIP(),
		"url":        s.SessionURL(),
		"expiresAt":  s.Sessions.ExpiresAt().Format(time.RFC3339),
	})
}

// selectInterface は複数 NIC からの選択(POST /admin/interface。DES-001)。
func (s *Server) selectInterface(w http.ResponseWriter, r *http.Request) {
	ip := r.PostFormValue("ip")
	for _, known := range s.IPs {
		if ip == known {
			s.mu.Lock()
			s.selectedIP = ip
			s.mu.Unlock()
			s.Log.Info("LAN インターフェースを %s に変更しました", ip)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"ok": true})
			return
		}
	}
	http.Error(w, "不明なインターフェースです", http.StatusBadRequest)
}
