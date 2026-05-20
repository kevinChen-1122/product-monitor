# 變數定義
BINARY_SCRAPER=scraper
BINARY_NOTIFIER=notifier
BINARY_SCHEDULER=scheduler
BINARY_STORAGE=storage
BIN_DIR=bin
CMD_DIR=cmd
SERVICES_DIR=services

# Go 參數
GO_CMD=go
GO_BUILD=$(GO_CMD) build
GO_CLEAN=$(GO_CMD) clean
GO_TIDY=$(GO_CMD) mod tidy

.PHONY: all tidy build build-scraper build-notifier build-scheduler build-storage up down logs clean help

# 預設目標
all: help

## tidy: 整理全域 Go modules
tidy:
	@echo "正在整理 Go modules..."
	$(GO_TIDY)

## build: 編譯所有服務
build: build-scraper build-notifier build-scheduler build-storage

build-scheduler: tidy
	@echo "正在編譯 scheduler..."
	$(GO_BUILD) -o $(BIN_DIR)/$(BINARY_SCHEDULER) ./$(CMD_DIR)/scheduler

build-storage:
	@echo "正在編譯 storage..."
	$(GO_BUILD) -o $(BIN_DIR)/$(BINARY_STORAGE) ./$(CMD_DIR)/storage

## build-scraper: 僅編譯 Scraper 服務
build-scraper: tidy
	@echo "正在編譯 Scraper..."
	$(GO_BUILD) -o $(BIN_DIR)/$(BINARY_SCRAPER) ./$(CMD_DIR)/scraper

## build-notifier: 僅編譯 Notifier 服務
build-notifier: tidy
	@echo "正在編譯 Notifier..."
	$(GO_BUILD) -o $(BIN_DIR)/$(BINARY_NOTIFIER) ./$(CMD_DIR)/notifier

## up: 啟動所有 Docker 容器 (含編譯)
up:
	@echo "正在啟動所有 Docker 服務..."
	docker-compose up -d --build

## down: 停止並移除所有 Docker 容器
down:
	@echo "正在停止 Docker 服務..."
	docker-compose down

## logs: 查看所有服務日誌 (或使用 make logs-s / make logs-n)
logs:
	docker-compose logs -f

## clean: 清理執行檔與編譯快取
clean:
	@echo "🧹 清理中..."
	rm -rf $(BIN_DIR)
	$(GO_CLEAN) -cache

## help: 顯示指令說明
help:
	@echo "可用指令:"
	@echo "  \033[36mmake tidy\033[0m           整理 Go 依賴"
	@echo "  \033[36mmake build\033[0m          編譯所有二進位檔 (bin/)"
	@echo "  \033[36mmake build-scraper\033[0m  僅編譯 Scraper"
	@echo "  \033[36mmake build-notifier\033[0m 僅編譯 Notifier"
	@echo "  \033[36mmake up\033[0m             使用 Docker Compose 啟動全環境"
	@echo "  \033[36mmake down\033[0m           停止 Docker 環境"
	@echo "  \033[36mmake logs\033[0m           查看容器日誌"
	@echo "  \033[36mmake clean\033[0m          刪除編譯產物"