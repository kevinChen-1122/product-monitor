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

// ScraperWorker 負責從 Redis 領取任務並調配瀏覽器執行爬取的 Worker 結構
type ScraperWorker struct {
	cfg            *config.Config
	redisStore     *store.RedisStore
	browserManager *engine.BrowserManager
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
			// 阻塞式從 Redis 中 Pop 任務關鍵字出來
			keyword, err := w.redisStore.PopTask(ctx)
			if err != nil {
				slog.Warn("[Scraper Worker] 領取任務暫時中斷 (可能正在等待任務)",
					"err_msg", err,
				)
				time.Sleep(2 * time.Second)
				continue
			}

			if err := w.redisStore.SetScrapeInflight(ctx, keyword); err != nil {
				slog.Warn("[Scraper Worker] 無法標記進行中關鍵字",
					"keyword", keyword,
					"err_msg", err,
				)
			}

			slog.Debug("[Scraper Worker] 領取到監控任務關鍵字，準備執行爬取...",
				"keyword", keyword,
			)

			products, err := w.scrapeWithSelfHealing(keyword)
			if clearErr := w.redisStore.ClearScrapeInflight(ctx); clearErr != nil {
				slog.Warn("[Scraper Worker] 無法清除進行中標記",
					"keyword", keyword,
					"err_msg", clearErr,
				)
			}

			if err != nil {
				slog.Error("[Scraper Worker] 爬取關鍵字失敗",
					"keyword", keyword,
					"err_msg", err,
				)
				continue
			}

			// 爬取成功，將整批結果序列化後塞入 Redis 商品隊列
			if len(products) > 0 {
				slog.Debug("[Scraper Worker] 關鍵字爬取成功，寫入 Redis 隊列...",
					"keyword", keyword,
					"product_quantity", len(products),
				)
				if err := w.redisStore.PushRawProducts(ctx, products); err != nil {
					slog.Error("[Scraper Worker] 將商品資料推入 Redis 失敗",
						"err_msg", err,
					)
				}
			} else {
				slog.Debug("[Scraper Worker] 關鍵字爬取完畢，但未發現符合條件的全新商品",
					"keyword", keyword,
				)
			}
		}
	}
}

const scrapeMaxAttempts = 3

// scrapeWithSelfHealing 執行爬取；遇瀏覽器連線中斷時重啟 Playwright 並重試
func (w *ScraperWorker) scrapeWithSelfHealing(keyword string) ([]models.Product, error) {
	var lastErr error
	for attempt := 1; attempt <= scrapeMaxAttempts; attempt++ {
		products, err := w.scrapeOnce(keyword)
		if err == nil {
			return products, nil
		}
		lastErr = err
		if !engine.IsRecoverableBrowserError(err) || attempt == scrapeMaxAttempts {
			return nil, err
		}
		slog.Warn("[Scraper Worker] 瀏覽器連線異常，準備重啟後重試",
			"keyword", keyword,
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

func (w *ScraperWorker) scrapeOnce(keyword string) ([]models.Product, error) {
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

	targetURL := parser.GetSearchURL(keyword)
	slog.Debug("[Scraper Worker] 正在載入搜尋頁面",
		"target_url", targetURL,
	)

	if _, err = page.Goto(targetURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(20000),
	}); err != nil {
		return nil, fmt.Errorf("頁面載入失敗: %w", err)
	}

	products, err := parser.ParseListings(page)
	if err != nil {
		return nil, fmt.Errorf("解析商品卡片失敗: %w", err)
	}
	return products, nil
}
