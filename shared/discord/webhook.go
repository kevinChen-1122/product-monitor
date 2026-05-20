package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Discord 規範：Webhook 單次請求最多只能帶 10 個 Embeds
const (
	MaxEmbedsPerMessage = 10
	maxRetries          = 2
)

// Embed 定義 (對應 Discord 的結構)
type Embed struct {
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	URL         string  `json:"url,omitempty"`
	Color       int     `json:"color,omitempty"`
	Fields      []Field `json:"fields,omitempty"`
	Image       *Image  `json:"image,omitempty"`
	Footer      *Footer `json:"footer,omitempty"`
}

type Field struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type Footer struct {
	Text string `json:"text"`
}

type Image struct {
	URL string `json:"url,omitempty"`
}

type WebhookPayload struct {
	Embeds []Embed `json:"embeds"`
}

// SendEmbeds 是對外公開的發送函式，會自動處理 10 筆拆單邏輯
func SendEmbeds(webhookURL string, embeds []Embed) error {
	if webhookURL == "" || len(embeds) == 0 {
		return nil
	}

	// 依照 Discord 限制，將過多的訊息拆成多個 HTTP 請求
	for i := 0; i < len(embeds); i += MaxEmbedsPerMessage {
		end := i + MaxEmbedsPerMessage
		if end > len(embeds) {
			end = len(embeds)
		}

		chunk := embeds[i:end]
		if err := sendPayloadWithRetries(webhookURL, chunk); err != nil {
			return err
		}
	}
	return nil
}

// sendPayloadWithRetries 負責底層 HTTP 傳輸，並處理 Discord 的 429 限速罰跪機制
func sendPayloadWithRetries(webhookURL string, embeds []Embed) error {
	payload := WebhookPayload{Embeds: embeds}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化 Discord 欄位失敗: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(body))
		if err != nil {
			return fmt.Errorf("建立 Discord HTTP 請求失敗: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)

		// 處理網路錯誤或請求失敗
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
			continue
		}

		// 處理 Discord 的 429 限速（Too Many Requests）
		if resp.StatusCode == http.StatusTooManyRequests {
			var rateResp struct {
				RetryAfter float64 `json:"retry_after"` // Discord 回傳秒數
			}
			_ = json.NewDecoder(resp.Body).Decode(&rateResp)
			resp.Body.Close()

			// 等待 Discord 指定的秒數，如果沒給時間，則按重試次數指數級遞增等待
			wait := time.Duration(rateResp.RetryAfter * float64(time.Second))
			if wait <= 0 {
				wait = time.Duration(attempt) * 3 * time.Second
			}

			time.Sleep(wait) // 在背景原地罰跪，不影響主執行緒運作
			continue
		}

		// 處理其他非成功狀態碼
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			lastErr = fmt.Errorf("discord 回傳錯誤狀態碼: %d", resp.StatusCode)
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
			continue
		}

		resp.Body.Close()
		return nil // 成功送達！
	}

	return fmt.Errorf("達到最大重試次數，最後錯誤: %w", lastErr)
}
