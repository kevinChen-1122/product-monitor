package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 存放全域共用設定
type Config struct {
	// 資料庫連線
	RedisAddr string
	MongoURI  string
	MongoDB   string
	MongoColl string

	// 爬蟲相關
	Keywords       []string
	ScrapeInterval time.Duration

	// 通知相關
	DiscordWebhookURLs     []string // 新商品通知（DISCORD_WEBHOOK_URL / DISCORD_WEBHOOK_URLS，逗號分隔，輪詢發送）
	DiscordAlertWebhookURL string   // slog Warn/Error 告警（未設定時沿用第一個 DiscordWebhookURLs）
	// DiscordNotifyBatchSize 單次 webhook 最多幾則 embed（1–10，Discord 上限 10）
	DiscordNotifyBatchSize int
	// DiscordNotifyBatchInterval 自第一筆進佇列起，最長等待多久再送出（與筆數上限擇一觸發）
	DiscordNotifyBatchInterval time.Duration

	// 系統設定
	AppMode string // e.g., "prod" or "dev"
}

// Load 會從環境變數讀取設定並回傳 Config 實例
func Load() *Config {
	// 讀取環境變數中的關鍵字 (格式預期為: iphone,ps5,macbook)
	kwStr := getEnv("KEYWORDS", "")
	var keywords []string
	if kwStr != "" {
		keywords = strings.Split(kwStr, ",")
		for i := range keywords {
			keywords[i] = strings.TrimSpace(keywords[i])
		}
	}

	// 讀取爬取間隔
	intervalStr := getEnv("SCRAPE_INTERVAL", "3m")
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		slog.Warn("無效的 SCRAPE_INTERVAL",
			"SCRAPE_INTERVAL", intervalStr,
		)
		interval = 3 * time.Minute
	}

	batchSize := 10
	if s := getEnv("NOTIFY_DISCORD_BATCH_SIZE", "10"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			batchSize = n
		}
	}
	if batchSize < 1 {
		batchSize = 1
	}
	if batchSize > 10 {
		batchSize = 10
	}

	batchIntervalStr := getEnv("NOTIFY_DISCORD_BATCH_INTERVAL", "5s")
	batchInterval, err := time.ParseDuration(batchIntervalStr)
	if err != nil {
		slog.Warn("無效的 NOTIFY_DISCORD_BATCH_INTERVAL",
			"NOTIFY_DISCORD_BATCH_INTERVAL", batchIntervalStr,
		)
		batchInterval = 5 * time.Second
	}
	if batchInterval < 500*time.Millisecond {
		batchInterval = 500 * time.Millisecond
	}

	return &Config{
		RedisAddr:                  getEnv("REDIS_ADDR", "localhost:6379"),
		MongoURI:                   getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:                    getEnv("MONGO_DB", "product_monitor"),
		MongoColl:                  getEnv("MONGO_COLLECTION", "products"),
		Keywords:                   keywords,
		ScrapeInterval:             interval,
		DiscordWebhookURLs:         loadDiscordWebhookURLs(),
		DiscordAlertWebhookURL:     getEnv("DISCORD_ALERT_WEBHOOK_URL", ""),
		DiscordNotifyBatchSize:     batchSize,
		DiscordNotifyBatchInterval: batchInterval,
		AppMode:                    getEnv("APP_MODE", "prod"),
	}
}

// getEnv 封裝讀取環境變數的邏輯，支援預設值
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func parseCommaList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func loadDiscordWebhookURLs() []string {
	seen := make(map[string]struct{})
	var out []string
	appendUnique := func(urls []string) {
		for _, u := range urls {
			if _, ok := seen[u]; ok {
				continue
			}
			seen[u] = struct{}{}
			out = append(out, u)
		}
	}
	appendUnique(parseCommaList(getEnv("DISCORD_WEBHOOK_URLS", "")))
	appendUnique(parseCommaList(getEnv("DISCORD_WEBHOOK_URL", "")))
	return out
}
