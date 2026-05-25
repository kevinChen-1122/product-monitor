package netutil

import (
	"context"
	"net"
	"net/http"
	"time"
)

// IPv4Client 強制走 tcp4，避免容器環境 DNS 回 IPv6 但無路由的問題。
var IPv4Client = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext(ctx, "tcp4", addr)
		},
		TLSHandshakeTimeout: 10 * time.Second,
	},
}
