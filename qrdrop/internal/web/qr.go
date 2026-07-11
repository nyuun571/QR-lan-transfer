package web

import (
	"net/http"

	qrcode "github.com/skip2/go-qrcode"
)

// qrPNG は現在のセッション URL の QR コード PNG を返す(DES-006。GET /admin/qr.png)。
// セッション再発行後は URL が変わるため、クライアント側でキャッシュさせない。
func (s *Server) qrPNG(w http.ResponseWriter, r *http.Request) {
	png, err := qrcode.Encode(s.SessionURL(), qrcode.Medium, 320)
	if err != nil {
		s.Log.Info("QR 生成に失敗: %v", err)
		http.Error(w, "QR コードを生成できませんでした", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(png)
}
