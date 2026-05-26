# --- 第一階段：編譯 Go 二進位檔 ---
FROM golang:1.24-bullseye AS builder
WORKDIR /app

RUN apt-get update && apt-get install -y git

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/scraper_bin ./cmd/scraper

# --- 第二階段：在 playwright 映像內安裝 Go playwright driver ---
# 使用已內建 Chromium 與 Node.js 的 playwright 映像，
# 只需下載小型 Go driver (<10MB)，不重新下載 Chromium。
FROM mcr.microsoft.com/playwright:v1.42.0-jammy AS pw-setup

COPY --from=golang:1.24-bullseye /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:$PATH
ENV GOPATH=/root/go

# 複製模組快取，避免重新下載 Go 依賴
COPY --from=builder /go/pkg/mod /root/go/pkg/mod
WORKDIR /app
COPY go.mod go.sum ./
COPY . .

# PLAYWRIGHT_BROWSERS_PATH 指向映像內建的 Chromium，
# install 指令只需下載 driver script，不觸發 browser 下載
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
RUN go run -mod=mod github.com/playwright-community/playwright-go/cmd/playwright install chromium

# --- 第三階段：最終運行層 ---
FROM mcr.microsoft.com/playwright:v1.42.0-jammy

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y tzdata ca-certificates && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

ENV TZ=Asia/Taipei
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright

WORKDIR /root/

COPY --from=pw-setup /root/.cache/ms-playwright-go /root/.cache/ms-playwright-go
COPY --from=builder /app/scraper_bin ./scraper

CMD ["./scraper"]
