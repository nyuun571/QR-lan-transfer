// Package transfer はファイル・テキストの送受信を担う(DES-003)。
// 共有一覧・受信一覧・テキストはメモリ保持のみで、アプリ終了時に消える(NFR-005)。
// ディスクに残るのは受信フォルダへ保存されたファイル本体だけである。
package transfer

import (
	"strconv"
	"sync"
	"time"
)

// Kind は項目種別。
type Kind string

const (
	KindFile Kind = "file"
	KindText Kind = "text"
)

// Item は転送項目(PC→スマホの共有、スマホ→PC の受信の両方に使う)。
type Item struct {
	ID         string    `json:"id"`
	Kind       Kind      `json:"kind"`
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	Text       string    `json:"text,omitempty"`
	Path       string    `json:"-"` // PC ローカル絶対パス(応答には含めない)
	ReceivedAt time.Time `json:"receivedAt"`
}

// Store は共有一覧・受信一覧と SSE 購読者を保持する。
type Store struct {
	mu       sync.Mutex
	nextID   int
	shared   []*Item // PC → スマホ
	received []*Item // スマホ → PC
	subs     map[chan struct{}]struct{}
}

// NewStore は空の Store を返す。
func NewStore() *Store {
	return &Store{subs: make(map[chan struct{}]struct{})}
}

func (s *Store) newID() string {
	s.nextID++
	return strconv.Itoa(s.nextID)
}

// AddShared は PC→スマホの共有項目を追加して返す。
func (s *Store) AddShared(it Item) *Item {
	s.mu.Lock()
	it.ID = s.newID()
	it.ReceivedAt = time.Now()
	p := &it
	s.shared = append(s.shared, p)
	s.mu.Unlock()
	s.notify()
	return p
}

// AddReceived はスマホ→PC の受信項目を追加して返す。
func (s *Store) AddReceived(it Item) *Item {
	s.mu.Lock()
	it.ID = s.newID()
	it.ReceivedAt = time.Now()
	p := &it
	s.received = append(s.received, p)
	s.mu.Unlock()
	s.notify()
	return p
}

// Shared は共有一覧のコピーを返す。
func (s *Store) Shared() []*Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Item, len(s.shared))
	copy(out, s.shared)
	return out
}

// Received は受信一覧のコピーを返す。
func (s *Store) Received() []*Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Item, len(s.received))
	copy(out, s.received)
	return out
}

// SharedByID は共有項目を ID で引く(なければ nil)。
func (s *Store) SharedByID(id string) *Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range s.shared {
		if it.ID == id {
			return it
		}
	}
	return nil
}

// Subscribe は変更通知チャネルを登録して返す(SSE 用)。
func (s *Store) Subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

// Unsubscribe は通知チャネルを解除する。
func (s *Store) Unsubscribe(ch chan struct{}) {
	s.mu.Lock()
	delete(s.subs, ch)
	s.mu.Unlock()
}

// notify は全購読者へ変更を通知する(詰まっている購読者はスキップ)。
func (s *Store) notify() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
