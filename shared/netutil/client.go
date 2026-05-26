package netutil

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// dohClient 直接以 IP 連到 Google Public DNS，完全不需要系統 DNS。
// URL 的 hostname 仍是 dns.google，TLS SNI 因此正確，憑證驗證不會失敗。
var dohClient = &http.Client{
	Timeout: 8 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			// 忽略傳入的 addr，強制連到 8.8.8.8:443
			return (&net.Dialer{Timeout: 5 * time.Second}).
				DialContext(ctx, "tcp4", "8.8.8.8:443")
		},
		TLSHandshakeTimeout: 5 * time.Second,
	},
}

type dohResponse struct {
	Answer []struct {
		Type int    `json:"type"` // 1 = A record
		Data string `json:"data"`
	} `json:"Answer"`
}

// resolveA 透過 DoH 查詢 hostname 的第一筆 IPv4 A 記錄。
func resolveA(ctx context.Context, hostname string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://dns.google/resolve?name="+hostname+"&type=A", nil)
	if err != nil {
		return "", err
	}
	resp, err := dohClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("DoH 查詢失敗: %w", err)
	}
	defer resp.Body.Close()

	var result dohResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("DoH 回應解析失敗: %w", err)
	}
	for _, ans := range result.Answer {
		if ans.Type == 1 {
			return ans.Data, nil
		}
	}
	return "", fmt.Errorf("DoH 找不到 %s 的 A 記錄", hostname)
}

// IPv4Client 透過 DoH 解析 DNS 後以 tcp4 連線，
// 徹底繞過容器內損壞的 UDP DNS resolver。
var IPv4Client = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			if net.ParseIP(host) == nil {
				ip, err := resolveA(ctx, host)
				if err != nil {
					return nil, fmt.Errorf("DNS 解析失敗 (%s): %w", host, err)
				}
				host = ip
			}
			return (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext(ctx, "tcp4", net.JoinHostPort(host, port))
		},
		TLSHandshakeTimeout: 10 * time.Second,
	},
}
