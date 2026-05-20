package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"product-monitor/shared/config"
	"product-monitor/shared/discord"
	"product-monitor/shared/logging"
	"product-monitor/shared/models"
	"product-monitor/shared/store"
)

func main() {
	cfg := config.Load()
	logging.Setup("notifier", cfg)

	slog.Info("[Notifier] 服務正在啟動...")

	webhookPool := discord.NewWebhookPool(cfg.DiscordWebhookURLs)
	if webhookPool.IsEmpty() {
		slog.Warn("[Notifier] 未設定 DISCORD_WEBHOOK_URL，商品通知將不會發送至 Discord")
	} else {
		slog.Info("[Notifier] 已載入 Discord webhook 輪詢池",
			"webhook_count", webhookPool.Len(),
		)
		for i := 0; i < webhookPool.Len(); i++ {
			slog.Info("[Notifier] webhook 池項目",
				"webhook_index", i,
				"webhook_tail", maskWebhookURL(webhookPool.URLAt(i)),
			)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancelHandle(cancel)

	rdb := store.NewRedisStore(cfg.RedisAddr)
	slog.Info("[Notifier] 開始監聽 Redis 通知隊列 [queue:notifier]...")

	runBatchDiscordWorker(ctx, cfg, rdb, webhookPool)
}

// runBatchDiscordWorker 從 Redis List 阻塞領取商品，累積後批次發送 Discord。
func runBatchDiscordWorker(ctx context.Context, cfg *config.Config, rdb *store.RedisStore, pool *discord.WebhookPool) {
	maxBatch := cfg.DiscordNotifyBatchSize
	interval := cfg.DiscordNotifyBatchInterval

	for {
		if ctx.Err() != nil {
			return
		}

		if pool.IsEmpty() {
			if n := drainNotifyQueue(ctx, rdb); n > 0 {
				slog.Warn("[Notifier] 未設定 webhook，已丟棄隊列中的商品",
					"product_quantity", n,
				)
			}
			time.Sleep(2 * time.Second)
			continue
		}

		first, err := rdb.PopNotifyProduct(ctx)
		if err != nil {
			if ctx.Err() != nil {
				slog.Info("[Notifier] 接收到停機訊號，停止監聽通知隊列")
				return
			}
			slog.Error("[Notifier] 從 Redis 領取通知失敗",
				"err_msg", err,
			)
			continue
		}

		batch := collectNotifyBatch(ctx, rdb, first, maxBatch, interval)
		if len(batch) == 0 {
			continue
		}

		flushDiscordBatch(pool, batch)
	}
}

func collectNotifyBatch(
	ctx context.Context,
	rdb *store.RedisStore,
	first models.Product,
	maxBatch int,
	interval time.Duration,
) []models.Product {
	batch := []models.Product{first}
	deadline := time.Now().Add(interval)

	for len(batch) < maxBatch {
		if ctx.Err() != nil {
			break
		}
		if time.Now().After(deadline) {
			break
		}

		p, ok, err := rdb.TryPopNotifyProduct(ctx)
		if err != nil {
			slog.Warn("[Notifier] 非阻塞領取通知失敗",
				"err_msg", err,
			)
			break
		}
		if !ok {
			break
		}
		batch = append(batch, p)
	}
	return batch
}

func flushDiscordBatch(pool *discord.WebhookPool, batch []models.Product) {
	idx, webhookURL := pool.Next()

	embeds := make([]discord.Embed, len(batch))
	for i := range batch {
		embeds[i] = productDiscordEmbed(batch[i])
	}

	slog.Debug("[Notifier] 準備送出商品通知批次",
		"webhook_index", idx,
		"product_quantity", len(batch),
	)

	if err := discord.SendEmbeds(webhookURL, embeds); err != nil {
		slog.Error("[Notifier] 發送 Discord 商品通知失敗",
			"err_msg", err,
			"webhook_index", idx,
			"webhook_tail", maskWebhookURL(webhookURL),
			"product_quantity", len(batch),
		)
		return
	}

	slog.Info("[Notifier] 已批次發送 Discord 商品通知",
		"webhook_index", idx,
		"webhook_tail", maskWebhookURL(webhookURL),
		"product_quantity", len(batch),
	)

	time.Sleep(1500 * time.Millisecond)
}

func drainNotifyQueue(ctx context.Context, rdb *store.RedisStore) int {
	n := 0
	for {
		_, ok, err := rdb.TryPopNotifyProduct(ctx)
		if err != nil || !ok {
			return n
		}
		n++
	}
}

func productDiscordEmbed(p models.Product) discord.Embed {
	localTime := p.CreatedAt.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")
	title := truncateRunes(p.Title, 250)
	return discord.Embed{
		Title:       title,
		Description: "",
		URL:         p.Link,
		Color:       3066993,
		Fields: []discord.Field{
			{Name: "💰 價格", Value: "```" + p.Price + "```", Inline: true},
			{Name: "👤 賣家名稱", Value: "```" + p.SellerName + "```", Inline: true},
		},
		Footer: &discord.Footer{
			Text: fmt.Sprintf("偵測時間：%s", localTime),
		},
	}
}

func maskWebhookURL(url string) string {
	if len(url) <= 12 {
		return "..."
	}
	return "..." + url[len(url)-12:]
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return string(r[:1])
	}
	return string(r[:max-1]) + "…"
}

func cancelHandle(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	cancel()
}
