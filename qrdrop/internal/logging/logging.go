// Package logging は動作ログのローカル出力を提供する(DES-007)。
// 接続元 IP・操作・ファイル名・結果・拒否理由のみを記録し、
// ファイル内容・テキスト本文は一切記録しない(NFR-004)。
package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Logger はファイルと標準エラーの両方へ書き出すロガー。
type Logger struct {
	file *os.File
	l    *log.Logger
}

// DefaultDir は既定のログ出力先(%APPDATA%\QRDrop\logs)を返す。
func DefaultDir() (string, error) {
	base, err := os.UserConfigDir() // Windows では %APPDATA%
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "QRDrop", "logs"), nil
}

// New は dir 配下に日付名のログファイルを作成して Logger を返す。
func New(dir string) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ログディレクトリを作成できません: %w", err)
	}
	name := filepath.Join(dir, "qrdrop-"+time.Now().Format("20060102")+".log")
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("ログファイルを開けません: %w", err)
	}
	return &Logger{
		file: f,
		l:    log.New(io.MultiWriter(f, os.Stderr), "", log.LstdFlags),
	}, nil
}

// Op は転送操作の結果を記録する(内容は記録しない)。
func (lg *Logger) Op(remote, op, name, result string) {
	lg.l.Printf("op=%s remote=%s name=%q result=%s", op, remote, name, result)
}

// Deny はアクセス拒否とその理由を記録する。
func (lg *Logger) Deny(remote, path, reason string) {
	lg.l.Printf("deny remote=%s path=%s reason=%s", remote, path, reason)
}

// Info は一般情報を記録する。
func (lg *Logger) Info(format string, args ...any) {
	lg.l.Printf(format, args...)
}

// Close はログファイルを閉じる。
func (lg *Logger) Close() error {
	return lg.file.Close()
}
