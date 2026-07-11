package main

import (
	"net"
	"strings"
	"testing"
)

func mustCIDR(t *testing.T, s string) net.Addr {
	t.Helper()
	ip, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	ipnet.IP = ip
	return ipnet
}

func TestFilterPrivateIPv4(t *testing.T) {
	addrs := []net.Addr{
		mustCIDR(t, "192.168.1.10/24"), // 採用
		mustCIDR(t, "10.20.30.40/8"),   // 採用
		mustCIDR(t, "203.0.113.7/24"),  // パブリック → 除外
		mustCIDR(t, "169.254.1.1/16"),  // リンクローカル → 除外
		mustCIDR(t, "127.0.0.1/8"),     // ループバック → 除外
		mustCIDR(t, "fe80::1/64"),      // IPv6 → 除外
		mustCIDR(t, "fd00::1/8"),       // IPv6 ULA → 除外(IPv4 のみ対象)
	}
	got := filterPrivateIPv4(addrs)
	want := []string{"192.168.1.10", "10.20.30.40"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

// TC-003: LAN 未接続時は「ネットワーク未接続」エラーで起動中断(REQ-001 異常系)
func TestCheckLanNoNetwork(t *testing.T) {
	err := checkLan(nil)
	if err == nil || !strings.Contains(err.Error(), "ネットワーク未接続") {
		t.Errorf("checkLan(nil) = %v, want ネットワーク未接続エラー", err)
	}
	if err := checkLan([]string{"192.168.1.3"}); err != nil {
		t.Errorf("LAN 検出時にエラー: %v", err)
	}
}

func TestListenWithFallback(t *testing.T) {
	// 先にポートを塞いでおき、次のポートへフォールバックすることを確認
	ln0, port0, err := listenWithFallback(38931, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer ln0.Close()
	ln1, port1, err := listenWithFallback(port0, 10)
	if err != nil {
		t.Fatalf("フォールバックに失敗: %v", err)
	}
	defer ln1.Close()
	if port1 == port0 {
		t.Errorf("同じポート %d を二重に確保した", port0)
	}
}
