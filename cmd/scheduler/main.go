package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"product-monitor/shared/config"
	"product-monitor/shared/logging"
	"product-monitor/shared/models"
	"product-monitor/shared/sheets"
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
	tasks, err := loadSearchTasks(ctx, cfg)
	if err != nil {
		slog.Error("[Scheduler] 無法取得搜尋任務清單",
			"err_msg", err,
		)
		return
	}

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

	if len(tasks) == 0 {
		slog.Warn("[Scheduler] 搜尋任務清單為空，略過派發")
		return
	}

	slog.Info("[Scheduler] 定時觸發，正在向 Redis 隊列派發搜尋任務",
		"source", taskSource(cfg),
		"task_count", len(tasks),
	)
	for _, task := range tasks {
		err := rdb.PushTask(ctx, task)
		if err != nil {
			slog.Warn("[Scheduler] 派發任務失敗",
				"keyword", task.Keyword,
				"price_start", task.PriceStart,
				"price_end", task.PriceEnd,
				"err_msg", err,
			)
		}
	}
	slog.Info("[Scheduler] 本輪搜尋任務派發完畢")
}

func loadSearchTasks(ctx context.Context, cfg *config.Config) ([]models.SearchTask, error) {
	csvURL := cfg.GoogleSheetExportURL()
	if csvURL != "" {
		return sheets.FetchSearchTasks(ctx, csvURL)
	}
	if len(cfg.Keywords) > 0 {
		slog.Warn("[Scheduler] 未設定 Google 試算表，使用環境變數 KEYWORDS 後備（價格 0–95000）")
		tasks := make([]models.SearchTask, 0, len(cfg.Keywords))
		for _, kw := range cfg.Keywords {
			kw = strings.TrimSpace(kw)
			if kw == "" {
				continue
			}
			tasks = append(tasks, models.SearchTask{
				Keyword:    kw,
				PriceStart: 0,
				PriceEnd:   95000,
			})
		}
		if len(tasks) > 0 {
			return tasks, nil
		}
	}
	return nil, fmt.Errorf("請設定 GOOGLE_SHEET_CSV_URL 或 GOOGLE_SHEETS_ID（或後備 KEYWORDS）")
}

func taskSource(cfg *config.Config) string {
	if cfg.GoogleSheetExportURL() != "" {
		return "google_sheet_csv"
	}
	return "env_keywords"
}

func cancelHandle(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	cancel()
}
