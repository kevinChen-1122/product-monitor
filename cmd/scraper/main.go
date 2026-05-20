package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"product-monitor/services/scraper"
	"product-monitor/services/scraper/engine"
	"product-monitor/shared/config"
	"product-monitor/shared/logging"
	"product-monitor/shared/store"
)

func main() {
	cfg := config.Load()
	logging.Setup("scraper", cfg)

	slog.Info("[Scraper] 服務初始化啟動中...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancelHandle(cancel)

	// 初始化 Redis 儲存庫
	rdb := store.NewRedisStore(cfg.RedisAddr)

	// 初始化 BrowserManager (在 Docker 內預設啟用 headless 無頭模式)
	bm, err := engine.NewBrowserManager(true)
	if err != nil {
		slog.Error("[Scraper] 啟動瀏覽器管理器失敗",
			"err_msg", err,
		)
	}
	defer bm.Close()

	// 建立並啟動你的 Worker 協程
	worker := scraper.NewScraperWorker(cfg, rdb, bm)
	worker.Start(ctx)
}

func cancelHandle(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	cancel()
}
