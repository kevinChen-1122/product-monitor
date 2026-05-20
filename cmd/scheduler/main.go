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
	dispatch(ctx, rdb, cfg.Keywords)

	for {
		select {
		case <-ctx.Done():
			slog.Info("[Scheduler] 接收到停機訊號，停止派發")
			return
		case <-ticker.C:
			dispatch(ctx, rdb, cfg.Keywords)
		}
	}
}

func dispatch(ctx context.Context, rdb *store.RedisStore, keywords []string) {
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
