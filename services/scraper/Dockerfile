# --- 第一階段：編譯與驅動準備 ---
FROM golang:1.24-bullseye AS builder
WORKDIR /app

# 安裝 Git 用於下載依賴
RUN apt-get update && apt-get install -y git

# 複製依賴文件並下載
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# 1. 編譯專案執行檔
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/scraper_bin ./cmd/scraper

# 2. 🚀【核心關鍵】利用 Go 官方命令，直接把 1.42.1 專用的驅動腳本下載到 /root/.cache
# 這會強迫環境在編譯期就生成 /root/.cache/ms-playwright-go/1.42.1/playwright.sh 這些檔案
RUN go run github.com/playwright-community/playwright-go/cmd/playwright install --with-deps chromium

# --- 第二階段：運行層 ---
FROM mcr.microsoft.com/playwright:v1.42.0-jammy

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y tzdata ca-certificates && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

ENV TZ=Asia/Taipei

WORKDIR /root/

# 3. 🚀【雙劍合璧】把第一階段下載好、熱騰騰的 Go 專用驅動，完整複製到運行層相同的坑位！
COPY --from=builder /root/.cache/ms-playwright-go /root/.cache/ms-playwright-go

# 4. 複製編譯好的執行檔
COPY --from=builder /app/scraper_bin ./scraper

CMD ["./scraper"]