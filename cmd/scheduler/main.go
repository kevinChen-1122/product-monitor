package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"product-monitor/shared/config"
	"product-monitor/shared/logging"
	"product-monitor/shared/store"
)

func main() {
	cfg := config.Load()
	logging.Setup("scheduler", cfg)

	slog.Info("[Scheduler] 服務正在啟動...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancelHandle(cancel)

	// 只需要連 Redis
	rdb := store.NewRedisStore(cfg.RedisAddr)
	slog.Info("[Scheduler] Redis 連線成功，開始計時派發任務...")

	// 建立定時器
	ticker := time.NewTicker(cfg.ScrapeInterval)
	defer ticker.Stop()

	// 啟動時先立刻派發第一次
	dispatch(ctx, rdb, cfg)

	for {
		select {
		case <-ctx.Done():
			slog.Info("[Scheduler] 接收到停機訊號，停止派發")
			return
		case <-ticker.C:
			dispatch(ctx, rdb, cfg)
		}
	}
}

func dispatch(ctx context.Context, rdb *store.RedisStore, cfg *config.Config) {
	keywords := cfg.Keywords

	queueLen, inflight, busy, err := rdb.ScrapeRoundStatus(ctx)
	if err != nil {
		slog.Error("[Scheduler] 無法檢查爬蟲輪次狀態",
			"err_msg", err,
		)
		return
	}
	if busy {
		attrs := []any{
			"queue_pending", queueLen,
			"scrape_interval", cfg.ScrapeInterval.String(),
		}
		if inflight != "" {
			attrs = append(attrs, "inflight_keyword", inflight)
		}
		// logging.Setup 已將 Warn 轉發至 DISCORD_ALERT_WEBHOOK_URL
		slog.Warn("[Scheduler] 上一輪爬蟲尚未完成，略過本次關鍵字派發", attrs...)
		return
	}

	if len(keywords) == 0 {
		slog.Warn("[Scheduler] 未設定 KEYWORDS，略過派發")
		return
	}

	slog.Info("[Scheduler] 定時觸發，正在向 Redis 隊列派發關鍵字",
		"keyword_count", len(keywords),
		"keywords", keywords,
	)
	for _, kw := range keywords {
		err := rdb.PushTask(ctx, kw)
		if err != nil {
			slog.Warn("[Scheduler] 派發關鍵字失敗",
				"keyword", kw,
				"err_msg", err,
			)
		}
	}
	slog.Info("[Scheduler] 本輪關鍵字派發完畢")
}

func cancelHandle(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	cancel()
}
