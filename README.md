# product-monitor
## 架構流程

```
Scheduler（定時 SCRAPE_INTERVAL）
    │
    ▼
從 Google 試算表 CSV 讀取關鍵字（每次派發前抓取，免 API 金鑰）
    │
    ▼
上一輪完成？（scraper:tasks 為空 且 scraper:inflight 不存在）
    │
    ├── 是 ──▶ LPUSH 全部 KEYWORDS ──▶ Redis: scraper:tasks
    │                                      │
    │                                      ▼ BRPop
    │                                 Scraper
    │                                 Set inflight → 爬取 Carousell → Clear inflight
    │                                      │
    │                                      ▼ LPUSH 商品批次
    │                                 Redis: queue:products
    │                                      │
    │                                      ▼ BRPop
    │                                 Storage
    │                                 SetNX 去重 (seen:{id}) → MongoDB 持久化
    │                                 新商品 ──▶ Redis: queue:notifier
    │                                      │
    │                                      ▼
    │                                 Notifier（批次累積 → Webhook 輪詢）
    │                                      │
    │                                      ▼
    │                                 Discord 新商品通知
    │
    └── 否 ──▶ 略過派發 ──▶ Discord 告警（DISCORD_ALERT_WEBHOOK_URL）
```

**上一輪完成條件**：`scraper:tasks` 為空，且 `scraper:inflight` 不存在（爬蟲異常時 inflight 30 分鐘後過期）。

## 目錄結構

```
product-monitor/
├── bin/
│   ├── notifier
│   ├── scheduler
│   ├── scraper
│   └── storage
├── cmd/
│   ├── notifier/
│   │   └── main.go
│   ├── scheduler/
│   │   └── main.go
│   ├── scraper/
│   │   └── main.go
│   └── storage/
│       └── main.go
├── data/
├── services/
│   └── scraper/
│       ├── Dockerfile
│       ├── engine/
│       │   └── browser.go
│       ├── parser/
│       │   └── carousell.go
│       └── worker.go
├── shared/
│   ├── config/
│   │   └── config.go
│   ├── discord/
│   │   ├── pool.go
│   │   └── webhook.go
│   ├── logging/
│   │   ├── handler.go
│   │   └── setup.go
│   ├── models/
│   │   └── product.go
│   └── store/
│       ├── mongodb.go
│       └── redis.go
├── .env
├── docker-compose.yml
├── Dockerfile.base
├── go.mod
├── go.sum
├── Makefile
└── README.md

```

### 目錄說明

| 路徑 | 說明 |
|------|------|
| `bin/` | `make build` 產出的二進位檔（可忽略版控） |
| `cmd/notifier/` | 從 Redis List `queue:notifier` 領取商品，批次打包後以 **webhook 輪詢池** 推送 Discord |
| `cmd/scheduler/` | 從 Google 試算表 CSV 讀取關鍵字，依 `SCRAPE_INTERVAL` 推入 Redis 任務隊列 |
| `shared/sheets/` | Google 試算表 CSV 匯出下載與關鍵字解析 |
| `cmd/scraper/` | 啟動 Playwright 瀏覽器與 Worker，領取任務並爬取 |
| `cmd/storage/` | 從 Redis 商品隊列取批、去重、寫入 MongoDB 並廣播新商品 |
| `data/` | 本地開發用資料目錄 |
| `services/scraper/` | 爬蟲業務邏輯：任務執行、瀏覽器自癒、Carousell 解析 |
| `shared/config/` | 從環境變數載入全域設定 |
| `shared/discord/` | Discord Webhook 共用發送 |
| `shared/logging/` | 統一 slog 設定；`Warn` / `Error` 可轉發至 Discord 告警 |
| `shared/models/` | 商品資料結構 |
| `shared/store/` | Redis（隊列、去重、Pub/Sub）與 MongoDB 存取 |

## 環境變數

於專案根目錄建立 `.env`（`docker-compose` 會自動載入）：

| 變數 | 說明 | 預設 |
|------|------|------|
| `APP_MODE` | `dev` 開啟 DEBUG 日誌；`prod` 僅 Info 以上 | `prod` |
| `DISCORD_ALERT_WEBHOOK_URL` | `slog` Warn/Error 告警 Webhook；未設定則沿用 `DISCORD_WEBHOOK_URL` | — |
| `DISCORD_WEBHOOK_URL` | 新商品 Discord Webhook；多個以逗號分隔，**每個商品** round-robin 輪詢 | — |
| `DISCORD_WEBHOOK_URLS` | 同上，可與 `DISCORD_WEBHOOK_URL` 並用（合併去重） | — |
| `GOOGLE_SHEET_CSV_URL` | 試算表 CSV 匯出網址或一般連結（Scheduler 關鍵字來源） | — |
| `GOOGLE_SHEETS_ID` | 試算表 ID（與 `GOOGLE_SHEETS_GID` 組匯出 URL，擇一於上者） | — |
| `GOOGLE_SHEETS_GID` | 工作表 gid | `0` |
| `KEYWORDS` | 後備：未設定試算表時，逗號分隔關鍵字（價格 0–95000） | — |
| `MONGO_URI` | MongoDB 連線字串 | `mongodb://localhost:27017` |
| `NOTIFY_DISCORD_BATCH_INTERVAL` | 自佇列收到第一筆起，最長等待多久即送出（與筆數上限擇一觸發） | `5s` |
| `NOTIFY_DISCORD_BATCH_SIZE` | 單次 webhook 最多幾則商品 embed（Discord 上限 10） | `10` |
| `REDIS_ADDR` | Redis 位址 | `localhost:6379` |
| `SCRAPE_INTERVAL` | Scheduler 派發間隔（如 `3m`、`5m`） | `3m` |

### Google 試算表設定（Scheduler 關鍵字）

1. 建立試算表：第一列標題 `keyword` | `price_start` | `price_end`（A/B/C 欄），以下每列一組搜尋條件。
2. **共用** → **知道連結的使用者均可檢視**（CSV 匯出才能免金鑰下載）。
3. 在 `.env` 設定 `GOOGLE_SHEET_CSV_URL`（貼試算表網址或匯出網址皆可），或設定 `GOOGLE_SHEETS_ID` + `GOOGLE_SHEETS_GID`。
4. CSV 匯出格式：`https://docs.google.com/spreadsheets/d/{ID}/export?format=csv&gid={GID}`

## 快速開始

```bash
# 編譯所有服務至 bin/
make build

# 啟動 Redis、MongoDB 與四個服務容器
make up

# 查看日誌
make logs

# 停止環境
make down
```

## Make 指令

| 指令 | 說明 |
|------|------|
| `make build` | 編譯 notifier、scheduler、scraper、storage |
| `make build-notifier` | 僅編譯 notifier |
| `make build-scraper` | 僅編譯 scraper |
| `make clean` | 刪除 `bin/` 與 Go 編譯快取 |
| `make down` | 停止並移除容器 |
| `make logs` | 追蹤所有容器日誌 |
| `make tidy` | 整理 Go modules |
| `make up` | `docker-compose up -d --build` |

## Docker 服務

`docker-compose.yml` 包含：

- `mongodb` — 商品持久化
- `notifier` — Discord 新商品通知（`Dockerfile.base`）
- `redis` — 任務隊列、商品隊列、通知隊列、去重
- `scheduler` — 定時派發關鍵字（`Dockerfile.base`）
- `scraper` — 爬蟲（`services/scraper/Dockerfile`，含 Playwright）
- `storage` — 儲存與廣播（`Dockerfile.base`）
