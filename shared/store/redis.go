package store

import (
	"context"
	"encoding/json"
	"product-monitor/shared/models"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	Client *redis.Client
}

func NewRedisStore(addr string) *RedisStore {
	return &RedisStore{
		Client: redis.NewClient(&redis.Options{
			Addr: addr,
		}),
	}
}

// PushTask 將關鍵字推入 List
func (r *RedisStore) PushTask(ctx context.Context, kw string) error {
	return r.Client.LPush(ctx, "scraper:tasks", kw).Err()
}

// PopTask 從 List 領取任務 (阻塞)
func (r *RedisStore) PopTask(ctx context.Context) (string, error) {
	res, err := r.Client.BRPop(ctx, 0, "scraper:tasks").Result()
	if err != nil {
		return "", err
	}
	return res[1], nil
}

// IsNew 使用 Redis SetNX 檢查商品是否已存在
func (r *RedisStore) IsNew(ctx context.Context, productID string) (bool, error) {
	key := "seen:" + productID
	// 設定 24 小時過期，避免 Redis 爆炸
	return r.Client.SetNX(ctx, key, "1", 30*24*time.Hour).Result()
}

// PushRawProducts 由 Scraper 呼叫：將爬到的整批商品序列化後塞入商品隊列
func (r *RedisStore) PushRawProducts(ctx context.Context, products []models.Product) error {
	payload, err := json.Marshal(products)
	if err != nil {
		return err
	}
	// 使用另一個 key 作為商品儲存隊列
	return r.Client.LPush(ctx, "queue:products", payload).Err()
}

// PopRawProducts 由 Storage 呼叫：從商品隊列中阻塞式取出整批商品
func (r *RedisStore) PopRawProducts(ctx context.Context) ([]models.Product, error) {
	// 阻塞 0 秒代表無限等待，直到有 Scraper 吐資料進來
	res, err := r.Client.BRPop(ctx, 0*time.Second, "queue:products").Result()
	if err != nil {
		return nil, err
	}
	// res[0] 是 key 名稱，res[1] 才是真正的 payload
	var products []models.Product
	err = json.Unmarshal([]byte(res[1]), &products)
	return products, err
}

const notifyQueueKey = "queue:notifier"

// EnqueueNotifyProduct 將待通知商品推入 Redis List（Notifier 消費）
func (r *RedisStore) EnqueueNotifyProduct(ctx context.Context, p models.Product) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return r.Client.LPush(ctx, notifyQueueKey, data).Err()
}

// PopNotifyProduct 阻塞領取一筆待通知商品（BRPop）
func (r *RedisStore) PopNotifyProduct(ctx context.Context) (models.Product, error) {
	res, err := r.Client.BRPop(ctx, 0, notifyQueueKey).Result()
	if err != nil {
		return models.Product{}, err
	}
	var p models.Product
	err = json.Unmarshal([]byte(res[1]), &p)
	return p, err
}

// TryPopNotifyProduct 非阻塞領取；無資料時 ok=false
func (r *RedisStore) TryPopNotifyProduct(ctx context.Context) (models.Product, bool, error) {
	res, err := r.Client.RPop(ctx, notifyQueueKey).Result()
	if err == redis.Nil {
		return models.Product{}, false, nil
	}
	if err != nil {
		return models.Product{}, false, err
	}
	var p models.Product
	if err := json.Unmarshal([]byte(res), &p); err != nil {
		return models.Product{}, false, err
	}
	return p, true, nil
}

// NotifyQueueLength 回傳待通知隊列長度（除錯用）
func (r *RedisStore) NotifyQueueLength(ctx context.Context) (int64, error) {
	return r.Client.LLen(ctx, notifyQueueKey).Result()
}
