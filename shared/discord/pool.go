package discord

import "sync/atomic"

// WebhookBatch 為輪詢分配後、要送往同一 webhook 的商品群組。
type WebhookBatch struct {
	Index    int
	URL      string
	Products int // 本批 embed 數量（由 notifier 填入後再送）
}

// WebhookPool 以 round-robin 輪詢多個 Discord webhook URL，分散單一頻道限流。
type WebhookPool struct {
	urls []string
	seq  atomic.Uint64
}

// NewWebhookPool 建立 pool；空白 URL 會略過。
func NewWebhookPool(urls []string) *WebhookPool {
	filtered := make([]string, 0, len(urls))
	for _, u := range urls {
		if u != "" {
			filtered = append(filtered, u)
		}
	}
	return &WebhookPool{urls: filtered}
}

func (p *WebhookPool) Len() int {
	if p == nil {
		return 0
	}
	return len(p.urls)
}

func (p *WebhookPool) IsEmpty() bool {
	return p.Len() == 0
}

// URLAt 回傳指定索引的 webhook（供啟動日誌等用途）。
func (p *WebhookPool) URLAt(index int) string {
	if p == nil || index < 0 || index >= len(p.urls) {
		return ""
	}
	return p.urls[index]
}

// Next 回傳下一個 webhook 的索引與 URL（執行緒安全輪詢）。
func (p *WebhookPool) Next() (index int, url string) {
	if p == nil || len(p.urls) == 0 {
		return -1, ""
	}
	i := p.seq.Add(1) - 1
	index = int(i) % len(p.urls)
	return index, p.urls[index]
}

// Pick 依 round-robin 為單一商品選定 webhook。
func (p *WebhookPool) Pick() WebhookBatch {
	index, url := p.Next()
	return WebhookBatch{Index: index, URL: url}
}
