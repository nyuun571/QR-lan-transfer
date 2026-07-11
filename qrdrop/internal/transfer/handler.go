package transfer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"qrdrop/internal/guard"
)

// MaxFileSize は1ファイルのアップロード上限(2GB。NFR-001/002)。
const MaxFileSize int64 = 2 << 30

// Logger は transfer が必要とするログ機能。
type Logger interface {
	Op(remote, op, name, result string)
	Info(format string, args ...any)
}

// Handler は転送系 HTTP ハンドラ群(DES-003)。
type Handler struct {
	Store   *Store
	RecvDir string // 受信フォルダ(既定: Downloads\QRDrop)
	Log     Logger

	// MaxSize はテストで上限を下げるためのフック(0 なら MaxFileSize)。
	MaxSize int64
}

func (h *Handler) maxSize() int64 {
	if h.MaxSize > 0 {
		return h.MaxSize
	}
	return MaxFileSize
}

// uniquePath は dir/name が存在する場合に "name (1).ext" 形式の空き名を返す(REQ-002)。
func uniquePath(dir, name string) string {
	p := filepath.Join(dir, name)
	if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
		return p
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		p = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			return p
		}
	}
}

// errTooLarge はサイズ上限超過。
var errTooLarge = errors.New("サイズ上限超過")

// saveFilePart は multipart のファイルパートを .part 経由で受信フォルダへ保存する。
// メモリに全量を載せず、ストリーミングで書き出す(§6: 中断時は .part を残さない)。
func (h *Handler) saveFilePart(part *multipart.Part, name string) (*Item, error) {
	dst := uniquePath(h.RecvDir, name)
	tmp := dst + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("一時ファイルを作成できません: %w", err)
	}
	// 上限+1 まで読んで超過を検知する
	n, err := io.Copy(f, io.LimitReader(part, h.maxSize()+1))
	closeErr := f.Close()
	if err == nil && closeErr != nil {
		err = closeErr
	}
	if err == nil && n > h.maxSize() {
		err = errTooLarge
	}
	if err != nil {
		os.Remove(tmp)
		return nil, err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return nil, fmt.Errorf("ファイルを保存できません: %w", err)
	}
	return h.Store.AddReceived(Item{
		Kind: KindFile,
		Name: filepath.Base(dst),
		Size: n,
		Path: dst,
	}), nil
}

// Upload は POST /s/<token>/upload(スマホ→PC。REQ-002)。
// multipart/form-data の "file" パート(複数可)と "text" フィールドを受け付ける。
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	if err := os.MkdirAll(h.RecvDir, 0o755); err != nil {
		h.Log.Op(r.RemoteAddr, "upload", "", "error: recv-dir: "+err.Error())
		http.Error(w, "PC 側で保存先フォルダを作成できませんでした。再試行してください。", http.StatusInternalServerError)
		return
	}
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "multipart 形式ではありません", http.StatusBadRequest)
		return
	}
	var saved []string
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			h.Log.Op(r.RemoteAddr, "upload", "", "error: "+err.Error())
			http.Error(w, "アップロードの受信に失敗しました。再試行してください。", http.StatusBadRequest)
			return
		}
		switch part.FormName() {
		case "file":
			name := guard.SanitizeFilename(part.FileName())
			it, err := h.saveFilePart(part, name)
			if errors.Is(err, errTooLarge) {
				h.Log.Op(r.RemoteAddr, "upload", name, "too-large")
				http.Error(w, "ファイルが大きすぎます(上限 2GB)", http.StatusRequestEntityTooLarge)
				return
			}
			if err != nil {
				h.Log.Op(r.RemoteAddr, "upload", name, "error: "+err.Error())
				http.Error(w, "PC 側の保存に失敗しました。再試行してください。", http.StatusInternalServerError)
				return
			}
			h.Log.Op(r.RemoteAddr, "upload", it.Name, "ok")
			saved = append(saved, it.Name)
		case "text":
			// テキストは 1MB まで(本文はログに残さない)
			data, err := io.ReadAll(io.LimitReader(part, 1<<20))
			if err != nil {
				http.Error(w, "テキストの受信に失敗しました", http.StatusBadRequest)
				return
			}
			text := string(data)
			if strings.TrimSpace(text) == "" {
				continue
			}
			h.Store.AddReceived(Item{Kind: KindText, Name: "テキスト", Size: int64(len(data)), Text: text})
			h.Log.Op(r.RemoteAddr, "upload-text", "", "ok")
			saved = append(saved, "(テキスト)")
		}
	}
	if len(saved) == 0 {
		http.Error(w, "送信する内容がありません", http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "saved": saved})
}

// Items は GET /s/<token>/items(スマホ向け共有一覧 JSON。REQ-003)。
func (h *Handler) Items(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"items": h.Store.Shared()})
}

// AdminItems は GET /admin/items(管理ページ向け。受信一覧+共有一覧)。
func (h *Handler) AdminItems(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"received": h.Store.Received(),
		"shared":   h.Store.Shared(),
	})
}

// Download は GET /s/<token>/files/<id>(PC→スマホ。REQ-003)。
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	it := h.Store.SharedByID(r.PathValue("id"))
	if it == nil || it.Kind != KindFile {
		http.Error(w, "見つかりません", http.StatusNotFound)
		return
	}
	f, err := os.Open(it.Path)
	if err != nil {
		h.Log.Op(r.RemoteAddr, "download", it.Name, "error: "+err.Error())
		http.Error(w, "PC 側でファイルを開けませんでした", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		http.Error(w, "PC 側でファイルを開けませんでした", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''`+urlEscape(it.Name))
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, it.Name, fi.ModTime(), f)
	h.Log.Op(r.RemoteAddr, "download", it.Name, "ok")
}

// TextFromPhone は POST /s/<token>/text(スマホ→PC のテキスト送信)。
func (h *Handler) TextFromPhone(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "テキストの受信に失敗しました", http.StatusBadRequest)
		return
	}
	text := r.PostFormValue("text")
	if strings.TrimSpace(text) == "" {
		http.Error(w, "テキストが空です", http.StatusBadRequest)
		return
	}
	h.Store.AddReceived(Item{Kind: KindText, Name: "テキスト", Size: int64(len(text)), Text: text})
	h.Log.Op(r.RemoteAddr, "upload-text", "", "ok")
	writeJSON(w, map[string]any{"ok": true})
}

// AdminShare は POST /admin/share(PC 側の共有追加。REQ-003)。
// ブラウザからは実ファイルパスを得られないため、内容をステージングフォルダ
// (受信フォルダ配下の .qrdrop-share)へ保存し、そこを配信元とする。
func (h *Handler) AdminShare(w http.ResponseWriter, r *http.Request) {
	stageDir := filepath.Join(h.RecvDir, ".qrdrop-share")
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		http.Error(w, "共有用フォルダを作成できませんでした", http.StatusInternalServerError)
		return
	}
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "multipart 形式ではありません", http.StatusBadRequest)
		return
	}
	added := 0
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			http.Error(w, "受信に失敗しました", http.StatusBadRequest)
			return
		}
		switch part.FormName() {
		case "file":
			name := guard.SanitizeFilename(part.FileName())
			dst := uniquePath(stageDir, name)
			f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				http.Error(w, "共有ファイルを保存できませんでした", http.StatusInternalServerError)
				return
			}
			n, err := io.Copy(f, io.LimitReader(part, h.maxSize()+1))
			closeErr := f.Close()
			if err == nil && closeErr != nil {
				err = closeErr
			}
			if err == nil && n > h.maxSize() {
				err = errTooLarge
			}
			if err != nil {
				os.Remove(dst)
				if errors.Is(err, errTooLarge) {
					http.Error(w, "ファイルが大きすぎます(上限 2GB)", http.StatusRequestEntityTooLarge)
					return
				}
				http.Error(w, "共有ファイルを保存できませんでした", http.StatusInternalServerError)
				return
			}
			h.Store.AddShared(Item{Kind: KindFile, Name: filepath.Base(dst), Size: n, Path: dst})
			h.Log.Op(r.RemoteAddr, "share", filepath.Base(dst), "ok")
			added++
		case "text":
			data, err := io.ReadAll(io.LimitReader(part, 1<<20))
			if err != nil {
				http.Error(w, "テキストの受信に失敗しました", http.StatusBadRequest)
				return
			}
			text := string(data)
			if strings.TrimSpace(text) == "" {
				continue
			}
			h.Store.AddShared(Item{Kind: KindText, Name: "テキスト", Size: int64(len(data)), Text: text})
			h.Log.Op(r.RemoteAddr, "share-text", "", "ok")
			added++
		}
	}
	if added == 0 {
		http.Error(w, "共有する内容がありません", http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// Events は SSE(GET /s/<token>/events, /admin/events)。一覧の変更を通知する(DES-005)。
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE 未対応", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch := h.Store.Subscribe()
	defer h.Store.Unsubscribe(ch)
	// 接続直後に1回通知(取りこぼし防止)
	fmt.Fprint(w, "data: update\n\n")
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			fmt.Fprint(w, "data: update\n\n")
			fl.Flush()
		}
	}
}

// CleanupPartFiles は受信フォルダに残った中断残骸(*.part)と
// 前回のステージングフォルダを起動時に削除する(§6)。
func CleanupPartFiles(recvDir string) {
	matches, _ := filepath.Glob(filepath.Join(recvDir, "*.part"))
	for _, m := range matches {
		os.Remove(m)
	}
	os.RemoveAll(filepath.Join(recvDir, ".qrdrop-share"))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

// urlEscape は RFC 5987 の filename* 用に UTF-8 名をパーセントエンコードする。
func urlEscape(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
		} else {
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0xf])
		}
	}
	return b.String()
}
