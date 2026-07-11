package main

import (
	"fmt"
	"net"

	"qrdrop/internal/guard"
)

// lanIPv4s は稼働中インターフェースのプライベート IPv4 アドレスを列挙する(DES-001)。
// ループバックは除く(スマホから届かないため)。
func lanIPv4s() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var ips []string
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		ips = append(ips, filterPrivateIPv4(addrs)...)
	}
	return ips, nil
}

// filterPrivateIPv4 はアドレス一覧からプライベート IPv4 のみを取り出す。
// (リンクローカル 169.254.x.x は QR 接続先として実用性がないため除外する)
func filterPrivateIPv4(addrs []net.Addr) []string {
	var out []string
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip4 := ipnet.IP.To4()
		if ip4 == nil || !guard.IsPrivate(ip4) || ip4.IsLinkLocalUnicast() || ip4.IsLoopback() {
			continue
		}
		out = append(out, ip4.String())
	}
	return out
}

// checkLan はプライベート IPv4 が検出されない場合に起動を中断するエラーを返す(REQ-001 異常系)。
func checkLan(ips []string) error {
	if len(ips) == 0 {
		return fmt.Errorf("ネットワーク未接続です。Wi-Fi または有線 LAN に接続してから再起動してください")
	}
	return nil
}

// listenWithFallback は port から順に最大 tries 個のポートを試して Listen する(DES-001)。
func listenWithFallback(port, tries int) (net.Listener, int, error) {
	for p := port; p < port+tries; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			return ln, p, nil
		}
	}
	return nil, 0, fmt.Errorf("ポート %d〜%d がすべて使用中です", port, port+tries-1)
}
