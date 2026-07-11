// Package session はワンタイムトークンによるセッション管理を提供する(DES-002)。
// トークンは crypto/rand 由来の 128bit を base64url 化したもので、
// TTL(既定 30 分)経過または手動失効で無効になる(REQ-001, REQ-005, NFR-002)。
package session

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

var (
	// ErrInvalid はトークン不一致。
	ErrInvalid = errors.New("無効なトークンです")
	// ErrExpired はトークンの期限切れ。
	ErrExpired = errors.New("トークンの有効期限が切れています")
)

// Manager は現在有効なセッション(常に1つ)を管理する。
type Manager struct {
	mu        sync.Mutex
	token     string
	createdAt time.Time
	expiresAt time.Time
	ttl       time.Duration

	// now はテストで時刻を差し替えるためのフック。
	now func() time.Time
}

// NewManager は TTL を指定して Manager を作り、初期トークンを発行する。
func NewManager(ttl time.Duration) *Manager {
	m := &Manager{ttl: ttl, now: time.Now}
	m.issue()
	return m
}

// issue は新しいトークンを発行する(呼び出し側でロックすること。初期化時を除く)。
func (m *Manager) issue() {
	b := make([]byte, 16) // 128bit
	if _, err := rand.Read(b); err != nil {
		// crypto/rand の失敗は継続不能(推測可能なトークンを発行してはならない)
		panic("crypto/rand の読み取りに失敗: " + err.Error())
	}
	m.token = base64.RawURLEncoding.EncodeToString(b)
	m.createdAt = m.now()
	m.expiresAt = m.createdAt.Add(m.ttl)
}

// Token は現在のトークンを返す。
func (m *Manager) Token() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.token
}

// ExpiresAt は現在のトークンの失効時刻を返す。
func (m *Manager) ExpiresAt() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.expiresAt
}

// Validate は与えられたトークンが有効か検査する。比較は定数時間で行う(§7-3)。
func (m *Manager) Validate(token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if subtle.ConstantTimeCompare([]byte(token), []byte(m.token)) != 1 {
		return ErrInvalid
	}
	if m.now().After(m.expiresAt) {
		return ErrExpired
	}
	return nil
}

// Revoke は現在のトークンを即時失効させ、新しいトークンを発行して返す(REQ-005)。
func (m *Manager) Revoke() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.issue()
	return m.token
}
