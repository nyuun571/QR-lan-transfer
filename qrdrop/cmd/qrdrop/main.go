// qrdrop は QR コードで接続する同一 LAN 内データ転送ツール(QRDrop)。
// 起動シーケンス(DES-001): LAN IPv4 検出 → 空きポート確保 → セッション発行 →
// HTTP サーバ開始 → 既定ブラウザで管理ページを開く。
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"qrdrop/internal/logging"
	"qrdrop/internal/session"
	"qrdrop/internal/transfer"
	"qrdrop/internal/web"
)

const (
	defaultPort = 8931
	portTries   = 100
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "エラー: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	port := flag.Int("port", defaultPort, "待ち受けポート(使用中なら自動で加算)")
	recvDir := flag.String("dir", "", `受信フォルダ(既定: %USERPROFILE%\Downloads\QRDrop)`)
	ttl := flag.Duration("ttl", 30*time.Minute, "セッションの有効期間")
	noBrowser := flag.Bool("no-browser", false, "起動時にブラウザを開かない")
	flag.Parse()

	// 受信フォルダ(REQ-002 既定: ダウンロード\QRDrop。質問票 Q3 案A)
	if *recvDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("ホームフォルダを特定できません: %w", err)
		}
		*recvDir = filepath.Join(home, "Downloads", "QRDrop")
	}

	logDir, err := logging.DefaultDir()
	if err != nil {
		return fmt.Errorf("ログフォルダを特定できません: %w", err)
	}
	lg, err := logging.New(logDir)
	if err != nil {
		return err
	}
	defer lg.Close()

	// LAN 未接続はエラー終了(REQ-001 異常系)
	ips, err := lanIPv4s()
	if err != nil {
		return fmt.Errorf("ネットワークインターフェースを取得できません: %w", err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("ネットワーク未接続です。Wi-Fi または有線 LAN に接続してから再起動してください")
	}

	ln, actualPort, err := listenWithFallback(*port, portTries)
	if err != nil {
		return err
	}

	// 中断残骸の削除(design-doc §6)
	transfer.CleanupPartFiles(*recvDir)

	sm := session.NewManager(*ttl)
	th := &transfer.Handler{Store: transfer.NewStore(), RecvDir: *recvDir, Log: lg}
	srv := web.NewServer(sm, th, lg, actualPort, ips)

	adminURL := fmt.Sprintf("http://127.0.0.1:%d/admin", actualPort)
	lg.Info("QRDrop 起動: port=%d recvDir=%s ips=%v", actualPort, *recvDir, ips)
	fmt.Printf("QRDrop を起動しました。\n  管理ページ: %s\n  受信フォルダ: %s\n終了するにはこのウィンドウで Ctrl+C を押してください。\n", adminURL, *recvDir)

	if !*noBrowser {
		if err := openBrowser(adminURL); err != nil {
			lg.Info("ブラウザを開けませんでした: %v(手動で %s を開いてください)", err, adminURL)
		}
	}

	return http.Serve(ln, srv.Handler())
}

// openBrowser は Windows の既定ブラウザで url を開く。
func openBrowser(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
