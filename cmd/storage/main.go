package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"product-monitor/shared/config"
	"product-monitor/shared/logging"
	"product-monitor/shared/models"
	"product-monitor/shared/store"
	"syscall"
	"time"
)

func main() {
	cfg := config.Load()
	logging.Setup("storage", cfg)

	slog.Info("[Storage] 服務正在啟動...")

	ctx, cancel := context.WithCancel(context.Background())
	go cancelHandle(cancel)

	// 需要連 Redis 和 MongoDB
	rdb := store.NewRedisStore(cfg.RedisAddr)
	db := store.NewMongoStore(cfg.MongoURI, cfg.MongoDB, cfg.MongoColl)
	defer func() {
		if err := db.Close(context.Background()); err != nil {
			slog.Warn("[Storage] 關閉 MongoDB 連線失敗", "err_msg", err)
		}
	}()
	slog.Info("[Storage] 資料庫連線完全就緒，開始監聽商品儲存隊列...")

	// 進入死迴圈，持續異步消化商品資料
	for {
		select {
		case <-ctx.Done():
			slog.Warn("[Storage] 接收到停機訊號，準備安全退出")
			return
		default:
			// 阻塞式領取 Scraper 爬到的整批商品
			products, err := rdb.PopRawProducts(ctx)
			if err != nil {
				slog.Error("[Storage] 領取商品資料錯誤",
					"err_msg", err,
				)
				continue
			}

			slog.Debug("[Storage] 收到一批來自爬蟲的商品，開始比對新舊...",
				"product_quantity", len(products),
			)

			// 比對 Redis 篩選出真正的新商品
			var newProducts []models.Product
			for i := range products {
				isNew, _ := rdb.IsNew(ctx, products[i].ID)
				if isNew {
					slog.Debug("[Storage] 發現未曾紀錄的新商品",
						"title", products[i].Title,
					)

					products[i].CreatedAt = time.Now()
					newProducts = append(newProducts, products[i])

					// 推入 Redis 通知隊列，由 Notifier 消費
					if err := rdb.EnqueueNotifyProduct(ctx, products[i]); err != nil {
						slog.Warn("[Storage] 推入通知隊列失敗",
							"title", products[i].Title,
							"err_msg", err,
						)
					}
				}
			}

			// 如果有新商品，一次性批次高速寫入 MongoDB
			if len(newProducts) > 0 {
				slog.Debug("[Storage] 正在將新商品批次寫入 MongoDB...",
					"product_quantity", len(newProducts),
				)
				if err := db.SaveProducts(ctx, newProducts); err != nil {
					slog.Error("[Storage] 批次寫入 MongoDB 失敗: %v",
						"err_msg", err,
					)
				} else {
					slog.Debug("[Storage] 成功持久化商品至資料庫",
						"product_quantity", len(newProducts),
					)
				}
			} else {
				slog.Debug("[Storage] 比對完成：此批次皆為重複商品，跳過寫入")
			}
		}
	}
}

func cancelHandle(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	cancel()
}
