package netutil

import (
	"context"
	"net"
	"net/http"
	"time"
)

// dnsDialer 強制走 tcp4，並直接向 8.8.8.8 查詢 DNS，
// 繞過容器內可能失效的 /etc/resolv.conf。
var dnsDialer = &net.Dialer{
	Timeout:   10 * time.Second,
	KeepAlive: 30 * time.Second,
	Resolver: &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).
				DialContext(ctx, "udp", "8.8.8.8:53")
		},
	},
}

// IPv4Client 強制走 tcp4 並使用 Google Public DNS，
// 避免容器環境 DNS 回 IPv6 或 DNS 查詢逾時的問題。
var IPv4Client = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return dnsDialer.DialContext(ctx, "tcp4", addr)
		},
		TLSHandshakeTimeout: 10 * time.Second,
	},
}
