package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	playwright "github.com/playwright-community/playwright-go"
	"product-monitor/services/scraper/engine"
	"product-monitor/services/scraper/parser"
	"product-monitor/shared/config"
	"product-monitor/shared/models"
	"product-monitor/shared/store"
)

// browserRestartInterval 每爬取這麼多頁就主動重啟 Chromium，防止記憶體長期累積。
// 設為略低於關鍵字總數，確保每輪結束後都能重置。
const browserRestartInterval = 40

// ScraperWorker 負責從 Redis 領取任務並調配瀏覽器執行爬取的 Worker 結構
type ScraperWorker struct {
	cfg            *config.Config
	redisStore     *store.RedisStore
	browserManager *engine.BrowserManager
	pagesScraped   int // 累計成功爬取頁數，用於觸發主動重啟
}

// NewScraperWorker 建立並初始化 Worker 實體
func NewScraperWorker(cfg *config.Config, rdb *store.RedisStore, bm *engine.BrowserManager) *ScraperWorker {
	return &ScraperWorker{
		cfg:            cfg,
		redisStore:     rdb,
		browserManager: bm,
	}
}

// Start 啟動無窮監聽迴圈，從 Redis 領取任務並執行爬蟲
func (w *ScraperWorker) Start(ctx context.Context) {
	slog.Info("[Scraper Worker] 服務已啟動，開始監聽 Redis 任務隊列...")

	for {
		select {
		case <-ctx.Done():
			slog.Warn("[Scraper Worker] 接收到停機訊號，準備安全退出...")
			return
		default:
			task, err := w.redisStore.PopTask(ctx)
			if err != nil {
				slog.Warn("[Scraper Worker] 領取任務暫時中斷 (可能正在等待任務)",
					"err_msg", err,
				)
				time.Sleep(2 * time.Second)
				continue
			}

			if err := w.redisStore.SetScrapeInflight(ctx, task.Keyword); err != nil {
				slog.Warn("[Scraper Worker] 無法標記進行中關鍵字",
					"keyword", task.Keyword,
					"err_msg", err,
				)
			}

			slog.Debug("[Scraper Worker] 領取到監控任務，準備執行爬取...",
				"keyword", task.Keyword,
				"price_start", task.PriceStart,
				"price_end", task.PriceEnd,
			)

			products, err := w.scrapeWithSelfHealing(ctx, task)
			if clearErr := w.redisStore.ClearScrapeInflight(ctx); clearErr != nil {
				slog.Warn("[Scraper Worker] 無法清除進行中標記",
					"keyword", task.Keyword,
					"err_msg", clearErr,
				)
			}

			if err != nil {
				slog.Error("[Scraper Worker] 爬取關鍵字失敗",
					"keyword", task.Keyword,
					"price_start", task.PriceStart,
					"price_end", task.PriceEnd,
					"err_msg", err,
				)
				continue
			}

			// 爬取成功，將整批結果序列化後塞入 Redis 商品隊列
			if len(products) > 0 {
				slog.Debug("[Scraper Worker] 關鍵字爬取成功，寫入 Redis 隊列...",
					"keyword", task.Keyword,
					"product_quantity", len(products),
				)
				if err := w.redisStore.PushRawProducts(ctx, products); err != nil {
					slog.Error("[Scraper Worker] 將商品資料推入 Redis 失敗",
						"err_msg", err,
					)
				}
			} else {
				slog.Debug("[Scraper Worker] 關鍵字爬取完畢，但未發現符合條件的全新商品",
					"keyword", task.Keyword,
				)
			}

			// 主動定期重啟瀏覽器，防止 Chromium 記憶體長期累積拖慢速度
			w.pagesScraped++
			if w.pagesScraped >= browserRestartInterval {
				w.pagesScraped = 0
				slog.Info("[Scraper Worker] 定期重啟瀏覽器以釋放記憶體",
					"pages_since_last_restart", browserRestartInterval,
				)
				if restartErr := w.browserManager.Recover(); restartErr != nil {
					slog.Warn("[Scraper Worker] 定期重啟瀏覽器失敗",
						"err_msg", restartErr,
					)
				}
			}
		}
	}
}

const scrapeMaxAttempts = 3

// scrapeWithSelfHealing 執行爬取；遇瀏覽器連線中斷時重啟 Playwright 並重試
func (w *ScraperWorker) scrapeWithSelfHealing(ctx context.Context, task models.SearchTask) ([]models.Product, error) {
	var lastErr error
	for attempt := 1; attempt <= scrapeMaxAttempts; attempt++ {
		products, err := w.scrapeOnce(ctx, task)
		if err == nil {
			return products, nil
		}
		lastErr = err
		if !engine.IsRecoverableBrowserError(err) || attempt == scrapeMaxAttempts {
			return nil, err
		}
		slog.Warn("[Scraper Worker] 瀏覽器連線異常，準備重啟後重試",
			"keyword", task.Keyword,
			"attempt", attempt,
			"err_msg", err,
		)
		if recoverErr := w.browserManager.Recover(); recoverErr != nil {
			return nil, fmt.Errorf("瀏覽器恢復失敗: %v (原始: %v)", recoverErr, err)
		}
		time.Sleep(2 * time.Second)
	}
	return nil, lastErr
}

func (w *ScraperWorker) scrapeOnce(ctx context.Context, task models.SearchTask) ([]models.Product, error) {
	if err := w.browserManager.EnsureBrowserAlive(); err != nil {
		return nil, fmt.Errorf("瀏覽器自癒檢查失敗: %w", err)
	}

	browserCtx, err := w.browserManager.NewContext()
	if err != nil {
		return nil, fmt.Errorf("建立分頁上下文失敗: %w", err)
	}
	defer browserCtx.Close()

	page, err := browserCtx.NewPage()
	if err != nil {
		return nil, fmt.Errorf("建立分頁失敗: %w", err)
	}
	defer page.Close()

	targetURL := parser.GetSearchURL(task.Keyword, task.PriceStart, task.PriceEnd)
	slog.Debug("[Scraper Worker] 正在載入搜尋頁面",
		"keyword", task.Keyword,
		"price_start", task.PriceStart,
		"price_end", task.PriceEnd,
		"target_url", targetURL,
	)

	// 封鎖圖片、字型、CSS：讓瀏覽器只跑 JS，顯著降低超時機率
	if err = page.Route("**/*", func(route playwright.Route) {
		switch route.Request().ResourceType() {
		case "image", "media", "font", "stylesheet":
			route.Abort()
		default:
			route.Continue()
		}
	}); err != nil {
		return nil, fmt.Errorf("設定資源封鎖失敗: %w", err)
	}

	// commit：server 回應一開始即觸發，不等 HTML 解析完畢
	if _, err = page.Goto(targetURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateCommit,
		Timeout:   playwright.Float(20000),
	}); err != nil {
		return nil, fmt.Errorf("頁面載入失敗: %w", err)
	}

	products, err := parser.ParseListings(ctx, page)
	if err != nil {
		return nil, fmt.Errorf("解析商品卡片失敗: %w", err)
	}
	return products, nil
}
