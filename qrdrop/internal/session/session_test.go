package session

import (
	"testing"
	"time"
)

func TestTokenFormat(t *testing.T) {
	m := NewManager(30 * time.Minute)
	tok := m.Token()
	// 128bit = 16byte → base64url(パディングなし)で 22 文字
	if len(tok) != 22 {
		t.Errorf("トークン長 = %d, want 22 (128bit): %q", len(tok), tok)
	}
	if m2 := NewManager(30 * time.Minute); m2.Token() == tok {
		t.Error("トークンが再現した(乱数でない疑い)")
	}
}

func TestValidate(t *testing.T) {
	m := NewManager(30 * time.Minute)
	if err := m.Validate(m.Token()); err != nil {
		t.Errorf("正しいトークンが拒否された: %v", err)
	}
	if err := m.Validate("wrong-token"); err != ErrInvalid {
		t.Errorf("誤ったトークンの結果 = %v, want ErrInvalid", err)
	}
	if err := m.Validate(""); err != ErrInvalid {
		t.Errorf("空トークンの結果 = %v, want ErrInvalid", err)
	}
}

func TestValidateExpired(t *testing.T) {
	m := NewManager(30 * time.Minute)
	base := time.Now()
	m.now = func() time.Time { return base.Add(31 * time.Minute) }
	if err := m.Validate(m.Token()); err != ErrExpired {
		t.Errorf("TTL 超過後の結果 = %v, want ErrExpired", err)
	}
	// 境界: TTL 以内なら有効
	m2 := NewManager(30 * time.Minute)
	m2.now = func() time.Time { return base.Add(29 * time.Minute) }
	if err := m2.Validate(m2.Token()); err != nil {
		t.Errorf("TTL 以内で拒否された: %v", err)
	}
}

func TestRevoke(t *testing.T) {
	m := NewManager(30 * time.Minute)
	old := m.Token()
	newTok := m.Revoke()
	if newTok == old {
		t.Error("Revoke 後もトークンが変わっていない")
	}
	if err := m.Validate(old); err != ErrInvalid {
		t.Errorf("旧トークンの結果 = %v, want ErrInvalid", err)
	}
	if err := m.Validate(newTok); err != nil {
		t.Errorf("新トークンが拒否された: %v", err)
	}
}
