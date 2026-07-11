package transfer

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type nopLog struct{}

func (nopLog) Op(remote, op, name, result string) {}
func (nopLog) Info(format string, args ...any)    {}

func newHandler(t *testing.T) *Handler {
	t.Helper()
	return &Handler{Store: NewStore(), RecvDir: t.TempDir(), Log: nopLog{}}
}

// multipartBody は file/text パートを組み立てる。
func multipartBody(t *testing.T, files map[string]string, text string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for name, content := range files {
		fw, err := w.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(fw, content)
	}
	if text != "" {
		w.WriteField("text", text)
	}
	w.Close()
	return &buf, w.FormDataContentType()
}

func doUpload(t *testing.T, h *Handler, body *bytes.Buffer, ctype string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/s/tok/upload", body)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	h.Upload(rec, req)
	return rec
}

func TestUploadSavesFile(t *testing.T) {
	h := newHandler(t)
	body, ctype := multipartBody(t, map[string]string{"photo.jpg": "JPEGDATA"}, "")
	rec := doUpload(t, h, body, ctype)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	data, err := os.ReadFile(filepath.Join(h.RecvDir, "photo.jpg"))
	if err != nil || string(data) != "JPEGDATA" {
		t.Fatalf("保存内容 = %q, err = %v", data, err)
	}
	got := h.Store.Received()
	if len(got) != 1 || got[0].Name != "photo.jpg" || got[0].Size != 8 {
		t.Errorf("受信一覧 = %+v", got[0])
	}
	// .part が残っていないこと
	if m, _ := filepath.Glob(filepath.Join(h.RecvDir, "*.part")); len(m) != 0 {
		t.Errorf(".part が残っている: %v", m)
	}
}

func TestUploadDuplicateNameGetsSuffix(t *testing.T) {
	h := newHandler(t)
	for i := 0; i < 3; i++ {
		body, ctype := multipartBody(t, map[string]string{"doc.txt": "v"}, "")
		if rec := doUpload(t, h, body, ctype); rec.Code != 200 {
			t.Fatalf("status = %d", rec.Code)
		}
	}
	for _, want := range []string{"doc.txt", "doc (1).txt", "doc (2).txt"} {
		if _, err := os.Stat(filepath.Join(h.RecvDir, want)); err != nil {
			t.Errorf("%s が存在しない: %v", want, err)
		}
	}
}

func TestUploadSanitizesTraversal(t *testing.T) {
	h := newHandler(t)
	body, ctype := multipartBody(t, map[string]string{`..\..\evil.txt`: "x"}, "")
	if rec := doUpload(t, h, body, ctype); rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(h.RecvDir, "evil.txt")); err != nil {
		t.Errorf("サニタイズ後の evil.txt が受信フォルダ内に無い: %v", err)
	}
	// 受信フォルダの外に書かれていないこと
	if _, err := os.Stat(filepath.Join(h.RecvDir, "..", "evil.txt")); err == nil {
		t.Error("受信フォルダの外にファイルが書かれた(パストラバーサル)")
	}
}

func TestUploadTooLarge(t *testing.T) {
	h := newHandler(t)
	h.MaxSize = 10 // テスト用に上限を 10 バイトへ
	body, ctype := multipartBody(t, map[string]string{"big.bin": strings.Repeat("A", 11)}, "")
	rec := doUpload(t, h, body, ctype)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rec.Code)
	}
	entries, _ := os.ReadDir(h.RecvDir)
	if len(entries) != 0 {
		t.Errorf("超過ファイルの残骸がある: %v", entries)
	}
}

func TestUploadText(t *testing.T) {
	h := newHandler(t)
	body, ctype := multipartBody(t, nil, "こんにちは")
	if rec := doUpload(t, h, body, ctype); rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	got := h.Store.Received()
	if len(got) != 1 || got[0].Kind != KindText || got[0].Text != "こんにちは" {
		t.Errorf("受信テキスト = %+v", got)
	}
}

func TestUploadEmpty(t *testing.T) {
	h := newHandler(t)
	body, ctype := multipartBody(t, nil, "")
	if rec := doUpload(t, h, body, ctype); rec.Code != http.StatusBadRequest {
		t.Errorf("空アップロードの status = %d, want 400", rec.Code)
	}
	// multipart ですらないボディ
	req := httptest.NewRequest("POST", "/s/tok/upload", strings.NewReader("junk"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	h.Upload(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("非 multipart の status = %d, want 400", rec.Code)
	}
}

func TestTextFromPhone(t *testing.T) {
	h := newHandler(t)
	req := httptest.NewRequest("POST", "/s/tok/text", strings.NewReader("text=hello"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.TextFromPhone(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	// 空テキストは 400
	req = httptest.NewRequest("POST", "/s/tok/text", strings.NewReader("text=++"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.TextFromPhone(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("空テキストの status = %d, want 400", rec.Code)
	}
}

func TestAdminShareAndDownload(t *testing.T) {
	h := newHandler(t)
	body, ctype := multipartBody(t, map[string]string{"report.pdf": "PDFDATA"}, "メモ")
	req := httptest.NewRequest("POST", "/admin/share", body)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	h.AdminShare(rec, req)
	if rec.Code != 200 {
		t.Fatalf("share status = %d, body = %s", rec.Code, rec.Body)
	}
	shared := h.Store.Shared()
	if len(shared) != 2 {
		t.Fatalf("共有件数 = %d, want 2(ファイル+テキスト)", len(shared))
	}

	// スマホ側 items JSON
	rec = httptest.NewRecorder()
	h.Items(rec, httptest.NewRequest("GET", "/s/tok/items", nil))
	var items struct {
		Items []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("items JSON: %v", err)
	}
	if len(items.Items) != 2 {
		t.Fatalf("items = %+v", items.Items)
	}

	// ダウンロード
	var fileID string
	for _, it := range items.Items {
		if it.Kind == "file" {
			fileID = it.ID
		}
	}
	req = httptest.NewRequest("GET", "/s/tok/files/"+fileID, nil)
	req.SetPathValue("id", fileID)
	rec = httptest.NewRecorder()
	h.Download(rec, req)
	if rec.Code != 200 || rec.Body.String() != "PDFDATA" {
		t.Errorf("download status = %d, body = %q", rec.Code, rec.Body)
	}

	// 存在しない ID は 404
	req = httptest.NewRequest("GET", "/s/tok/files/999", nil)
	req.SetPathValue("id", "999")
	rec = httptest.NewRecorder()
	h.Download(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("不明 ID の status = %d, want 404", rec.Code)
	}
}

func TestCleanupPartFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "broken.jpg.part"), []byte("x"), 0o600)
	os.WriteFile(filepath.Join(dir, "keep.jpg"), []byte("x"), 0o600)
	os.MkdirAll(filepath.Join(dir, ".qrdrop-share"), 0o755)
	CleanupPartFiles(dir)
	if _, err := os.Stat(filepath.Join(dir, "broken.jpg.part")); err == nil {
		t.Error(".part が削除されていない")
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.jpg")); err != nil {
		t.Error("通常ファイルまで削除された")
	}
	if _, err := os.Stat(filepath.Join(dir, ".qrdrop-share")); err == nil {
		t.Error("ステージングフォルダが削除されていない")
	}
}

func TestStoreNotify(t *testing.T) {
	s := NewStore()
	ch := s.Subscribe()
	defer s.Unsubscribe(ch)
	s.AddShared(Item{Kind: KindText, Name: "テキスト", Text: "x"})
	select {
	case <-ch:
	default:
		t.Error("AddShared で通知が来ていない")
	}
}
