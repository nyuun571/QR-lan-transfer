package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewCreatesDirAndFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "logs")
	lg, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer lg.Close()

	lg.Op("192.168.1.20:50000", "upload", "photo.jpg", "ok")
	lg.Deny("8.8.8.8:1234", "/s/abc", "public-ip")

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("ログファイルが1つ作成されるはず: entries=%v err=%v", entries, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(data)
	for _, want := range []string{"op=upload", `name="photo.jpg"`, "result=ok", "deny remote=8.8.8.8:1234", "reason=public-ip"} {
		if !strings.Contains(s, want) {
			t.Errorf("ログに %q が含まれていない。内容:\n%s", want, s)
		}
	}
}
