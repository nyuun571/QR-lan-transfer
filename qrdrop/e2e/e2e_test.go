// Package e2e は実バイナリを起動して受け入れ基準を検証する E2E テスト(P4)。
// 対応 TC は docs/agent-work/test/test-plan.md を参照。
// スマホ実機の代替として、QR デコード(TC-002)と LAN IP 経由の HTTP 要求で検証する。
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// instance は起動済みの qrdrop プロセス。
type instance struct {
	cmd     *exec.Cmd
	port    int
	recvDir string
	token   string
	lanIP   string
}

var exePath string

func TestMain(m *testing.M) {
	// 一度だけビルドして全テストで使い回す
	dir, err := os.MkdirTemp("", "qrdrop-e2e")
	if err != nil {
		panic(err)
	}
	exePath = filepath.Join(dir, "qrdrop.exe")
	cmd := exec.Command("go", "build", "-o", exePath, "qrdrop/cmd/qrdrop")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		panic(fmt.Sprintf("build failed: %v\n%s", err, out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// start は qrdrop を起動し、管理ページが応答するまで待つ。
func start(t *testing.T, ttl string, recvDir string) *instance {
	t.Helper()
	if recvDir == "" {
		recvDir = t.TempDir()
	}
	port := freePort(t)
	cmd := exec.Command(exePath, "-no-browser", "-port", fmt.Sprint(port), "-dir", recvDir, "-ttl", ttl)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ins := &instance{cmd: cmd, port: port, recvDir: recvDir}
	t.Cleanup(func() { ins.stop() })

	// 起動待ち(最大 10 秒)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("起動がタイムアウトしました")
		}
		resp, err := http.Get(ins.adminURL("/admin/info"))
		if err == nil {
			var info struct {
				URL        string `json:"url"`
				SelectedIP string `json:"selectedIp"`
			}
			json.NewDecoder(resp.Body).Decode(&info)
			resp.Body.Close()
			u, err := url.Parse(info.URL)
			if err != nil || info.URL == "" {
				t.Fatalf("info.url が不正: %q", info.URL)
			}
			ins.token = strings.TrimPrefix(u.Path, "/s/")
			ins.lanIP = info.SelectedIP
			return ins
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (i *instance) stop() {
	if i.cmd.Process != nil {
		i.cmd.Process.Kill()
		i.cmd.Wait()
	}
}

func (i *instance) adminURL(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", i.port, path)
}

// lanURL は LAN IP 経由の URL(スマホからのアクセス相当。接続元も LAN IP になる)。
func (i *instance) lanURL(path string) string {
	return fmt.Sprintf("http://%s:%d%s", i.lanIP, i.port, path)
}

func (i *instance) sessionPath(sub string) string {
	return "/s/" + i.token + sub
}

func getBody(t *testing.T, url string) (int, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// uploadFile は multipart で name/content を /upload へ送る。
func uploadFile(t *testing.T, ins *instance, name string, content []byte) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(content)
	w.Close()
	resp, err := http.Post(ins.lanURL(ins.sessionPath("/upload")), w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func adminItems(t *testing.T, ins *instance) (received, shared []map[string]any) {
	t.Helper()
	_, body := getBody(t, ins.adminURL("/admin/items"))
	var data struct {
		Received []map[string]any `json:"received"`
		Shared   []map[string]any `json:"shared"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatalf("items JSON: %v (%s)", err, body)
	}
	return data.Received, data.Shared
}

// decodeQR は /admin/qr.png をデコードして中身の文字列を返す(TC-002)。
func decodeQR(t *testing.T, ins *instance) string {
	t.Helper()
	status, body := getBody(t, ins.adminURL("/admin/qr.png"))
	if status != 200 {
		t.Fatalf("qr.png status = %d", status)
	}
	img, err := png.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("PNG デコード失敗: %v", err)
	}
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		t.Fatal(err)
	}
	res, err := qrcode.NewQRCodeReader().Decode(bmp, nil)
	if err != nil {
		t.Fatalf("QR デコード失敗: %v", err)
	}
	return res.GetText()
}

// TC-001: 起動 → 管理ページ + QR 表示
func TestTC001AdminPageAndQR(t *testing.T) {
	ins := start(t, "30m", "")
	status, body := getBody(t, ins.adminURL("/admin"))
	if status != 200 || !strings.Contains(string(body), "QRDrop 管理ページ") {
		t.Errorf("管理ページ: status=%d", status)
	}
	_, png := getBody(t, ins.adminURL("/admin/qr.png"))
	if !bytes.HasPrefix(png, []byte("\x89PNG")) {
		t.Error("qr.png が PNG でない")
	}
}

// TC-002: QR デコード → 転送ページ(スマホ読取の代替)
func TestTC002QRLeadsToTransferPage(t *testing.T) {
	ins := start(t, "30m", "")
	qrURL := decodeQR(t, ins)
	want := ins.lanURL(ins.sessionPath(""))
	if qrURL != want {
		t.Fatalf("QR の中身 = %q, want %q", qrURL, want)
	}
	status, body := getBody(t, qrURL)
	if status != 200 || !strings.Contains(string(body), "QRDrop 転送ページ") {
		t.Errorf("転送ページ: status=%d", status)
	}
}

// TC-004: ファイル送信 → 受信フォルダ保存 + 一覧反映
// TC-015: パストラバーサル名の封じ込め
func TestTC004UploadSavedAndListed(t *testing.T) {
	ins := start(t, "30m", "")
	content := []byte("e2e-upload-content")
	if status, body := uploadFile(t, ins, "photo.jpg", content); status != 200 {
		t.Fatalf("upload status=%d body=%s", status, body)
	}
	saved, err := os.ReadFile(filepath.Join(ins.recvDir, "photo.jpg"))
	if err != nil || !bytes.Equal(saved, content) {
		t.Errorf("保存内容不一致: %v", err)
	}
	received, _ := adminItems(t, ins)
	if len(received) != 1 || received[0]["name"] != "photo.jpg" {
		t.Errorf("received = %v", received)
	}

	// TC-015: トラバーサル名
	if status, _ := uploadFile(t, ins, `..\..\evil.txt`, []byte("x")); status != 200 {
		t.Fatal("トラバーサル名 upload が失敗")
	}
	if _, err := os.Stat(filepath.Join(ins.recvDir, "evil.txt")); err != nil {
		t.Error("evil.txt が受信フォルダ内に保存されていない")
	}
	if _, err := os.Stat(filepath.Join(ins.recvDir, "..", "evil.txt")); err == nil {
		t.Error("受信フォルダの外に書かれた(パストラバーサル)")
	}
}

// TC-005: テキスト送信 → 管理側一覧に全文
func TestTC005TextToPC(t *testing.T) {
	ins := start(t, "30m", "")
	text := "テスト用テキスト E2E"
	resp, err := http.PostForm(ins.lanURL(ins.sessionPath("/text")), url.Values{"text": {text}})
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("text post: %v status=%v", err, resp)
	}
	resp.Body.Close()
	received, _ := adminItems(t, ins)
	if len(received) != 1 || received[0]["text"] != text {
		t.Errorf("received = %v", received)
	}
}

// TC-006: 同名再送 → 連番保存
func TestTC006DuplicateNames(t *testing.T) {
	ins := start(t, "30m", "")
	uploadFile(t, ins, "doc.txt", []byte("v1"))
	uploadFile(t, ins, "doc.txt", []byte("v2"))
	b1, err1 := os.ReadFile(filepath.Join(ins.recvDir, "doc.txt"))
	b2, err2 := os.ReadFile(filepath.Join(ins.recvDir, "doc (1).txt"))
	if err1 != nil || err2 != nil || string(b1) != "v1" || string(b2) != "v2" {
		t.Errorf("連番保存が不正: %q(%v) / %q(%v)", b1, err1, b2, err2)
	}
}

// TC-007: PC→スマホ共有(SSE 通知 → items → download)
func TestTC007ShareToPhone(t *testing.T) {
	ins := start(t, "30m", "")

	// SSE 購読(初期イベントを読み捨ててから共有を追加)
	sseResp, err := http.Get(ins.lanURL(ins.sessionPath("/events")))
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()
	sseCh := make(chan string, 4)
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := sseResp.Body.Read(buf)
			if n > 0 {
				sseCh <- string(buf[:n])
			}
			if err != nil {
				close(sseCh)
				return
			}
		}
	}()
	waitSSE := func(why string) {
		select {
		case <-sseCh:
		case <-time.After(5 * time.Second):
			t.Fatalf("SSE 通知が 5 秒以内に届かない(%s)", why)
		}
	}
	waitSSE("initial")

	// PC 側で共有(ファイル+テキスト)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", "share.bin")
	fileContent := []byte("shared-file-content")
	fw.Write(fileContent)
	w.WriteField("text", "PC からのメモ")
	w.Close()
	resp, err := http.Post(ins.adminURL("/admin/share"), w.FormDataContentType(), &buf)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("share: %v %v", err, resp)
	}
	resp.Body.Close()

	waitSSE("after share") // REQ-003「数秒以内に現れる」

	// スマホ側 items
	_, body := getBody(t, ins.lanURL(ins.sessionPath("/items")))
	var data struct {
		Items []struct {
			ID, Kind, Name, Text string
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &data); err != nil || len(data.Items) != 2 {
		t.Fatalf("items = %s (err=%v)", body, err)
	}
	var fileID string
	for _, it := range data.Items {
		if it.Kind == "file" {
			fileID = it.ID
		} else if it.Text != "PC からのメモ" {
			t.Errorf("テキスト全文不一致: %q", it.Text)
		}
	}

	// ダウンロード内容一致
	status, dl := getBody(t, ins.lanURL(ins.sessionPath("/files/"+fileID)))
	if status != 200 || !bytes.Equal(dl, fileContent) {
		t.Errorf("download status=%d 内容一致=%v", status, bytes.Equal(dl, fileContent))
	}
}

// TC-010: トークンなし URL は 403(/admin はループバックのみ許可)
func TestTC010NoTokenBlocked(t *testing.T) {
	ins := start(t, "30m", "")
	if status, _ := getBody(t, ins.lanURL("/")); status != 403 {
		t.Errorf("`/` status = %d, want 403", status)
	}
	if status, _ := getBody(t, ins.lanURL("/s/")); status != 403 {
		t.Errorf("`/s/` status = %d, want 403", status)
	}
	if status, _ := getBody(t, ins.lanURL("/admin")); status != 403 {
		t.Errorf("LAN IP 経由 /admin status = %d, want 403", status)
	}
	if status, _ := getBody(t, ins.adminURL("/admin")); status != 200 {
		t.Errorf("ループバック /admin status = %d, want 200", status)
	}
}

// TC-011: TTL 失効 → 403 + 再読取案内
func TestTC011TTLExpiry(t *testing.T) {
	ins := start(t, "2s", "")
	url := ins.lanURL(ins.sessionPath(""))
	if status, _ := getBody(t, url); status != 200 {
		t.Fatalf("失効前 status = %d", status)
	}
	time.Sleep(3 * time.Second)
	status, body := getBody(t, url)
	if status != 403 || !strings.Contains(string(body), "読み取り直して") {
		t.Errorf("失効後 status = %d, 案内文含む = %v", status, strings.Contains(string(body), "読み取り直して"))
	}
}

// TC-012: 手動失効 → 旧 URL 403・新 URL 有効・QR 更新
func TestTC012RevokeAndReissue(t *testing.T) {
	ins := start(t, "30m", "")
	oldURL := ins.lanURL(ins.sessionPath(""))
	resp, err := http.Post(ins.adminURL("/admin/session/revoke"), "", nil)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("revoke: %v", err)
	}
	resp.Body.Close()

	if status, _ := getBody(t, oldURL); status != 403 {
		t.Errorf("旧 URL status = %d, want 403", status)
	}
	newQR := decodeQR(t, ins)
	if newQR == oldURL {
		t.Error("QR が旧 URL のまま")
	}
	if status, _ := getBody(t, newQR); status != 200 {
		t.Errorf("新 QR の URL status = %d, want 200", status)
	}
}

// TC-014: 100MB 転送が 30 秒以内(NFR-001)
func TestTC014Performance100MB(t *testing.T) {
	ins := start(t, "30m", "")
	big := bytes.Repeat([]byte("0123456789ABCDEF"), 100*1024*1024/16) // 100MB
	begin := time.Now()
	status, body := uploadFile(t, ins, "big.bin", big)
	elapsed := time.Since(begin)
	if status != 200 {
		t.Fatalf("upload status=%d body=%s", status, body)
	}
	if elapsed > 30*time.Second {
		t.Errorf("100MB 転送に %v かかった(30 秒超過)", elapsed)
	}
	fi, err := os.Stat(filepath.Join(ins.recvDir, "big.bin"))
	if err != nil || fi.Size() != int64(len(big)) {
		t.Errorf("保存サイズ = %v, want %d", fi, len(big))
	}
	t.Logf("TC-014 実測: 100MB を %v で転送(ローカルホスト内)", elapsed)
}

// TC-016: 再起動で一覧は消え、受信ファイルは残る(NFR-005)
func TestTC016NoPersistence(t *testing.T) {
	recvDir := t.TempDir()
	ins := start(t, "30m", recvDir)
	uploadFile(t, ins, "keep.txt", []byte("keep"))
	resp, _ := http.PostForm(ins.lanURL(ins.sessionPath("/text")), url.Values{"text": {"消えるテキスト"}})
	resp.Body.Close()
	received, _ := adminItems(t, ins)
	if len(received) != 2 {
		t.Fatalf("前提: received = %d 件, want 2", len(received))
	}
	ins.stop()

	ins2 := start(t, "30m", recvDir)
	received, shared := adminItems(t, ins2)
	if len(received) != 0 || len(shared) != 0 {
		t.Errorf("再起動後に一覧が残っている: received=%d shared=%d", len(received), len(shared))
	}
	if _, err := os.Stat(filepath.Join(recvDir, "keep.txt")); err != nil {
		t.Error("受信ファイル本体が消えた")
	}
}

// TC-017: ログに操作記録あり・本文なし(NFR-004)
func TestTC017LogPolicy(t *testing.T) {
	ins := start(t, "30m", "")
	marker := fmt.Sprintf("himitsu-marker-%d", time.Now().UnixNano())
	resp, err := http.PostForm(ins.lanURL(ins.sessionPath("/text")), url.Values{"text": {marker}})
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("text post: %v", err)
	}
	resp.Body.Close()
	ins.stop() // ログをフラッシュさせる

	appdata, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(appdata, "QRDrop", "logs", "qrdrop-"+time.Now().Format("20060102")+".log")
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("ログファイルが読めない: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "op=upload-text") {
		t.Error("ログに upload-text の操作記録がない")
	}
	if strings.Contains(s, marker) {
		t.Error("ログにテキスト本文が記録されている(NFR-004 違反)")
	}
}
