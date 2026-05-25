package sheets

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"product-monitor/shared/models"
	"product-monitor/shared/netutil"
)

const (
	colKeyword    = 0 // A 欄
	colPriceStart = 1 // B 欄
	colPriceEnd   = 2 // C 欄

	defaultPriceStart = 0
	defaultPriceEnd   = 95000
)

// FetchSearchTasks 從 Google 試算表 CSV 讀取搜尋任務。
// 固定欄位：A=keyword、B=price_start、C=price_end（第一列為標題）。
func FetchSearchTasks(ctx context.Context, csvURL string) ([]models.SearchTask, error) {
	if csvURL == "" {
		return nil, fmt.Errorf("未設定 Google 試算表 CSV URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, csvURL, nil)
	if err != nil {
		return nil, fmt.Errorf("建立請求失敗: %w", err)
	}

	resp, err := netutil.IPv4Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下載試算表 CSV 失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("試算表 CSV 回應狀態碼: %d", resp.StatusCode)
	}

	return parseSearchTasksCSV(resp.Body)
}

func parseSearchTasksCSV(r io.Reader) ([]models.SearchTask, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("解析 CSV 失敗: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("試算表至少需要標題列與一筆資料")
	}

	seen := make(map[string]struct{})
	var out []models.SearchTask
	for i := 1; i < len(records); i++ {
		task, ok, err := rowToSearchTask(records[i])
		if err != nil {
			return nil, fmt.Errorf("第 %d 列: %w", i+1, err)
		}
		if !ok {
			continue
		}
		key := taskDedupeKey(task)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, task)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("試算表中找不到有效搜尋任務（A=keyword, B=price_start, C=price_end）")
	}
	return out, nil
}

func rowToSearchTask(row []string) (models.SearchTask, bool, error) {
	keyword := cellAt(row, colKeyword)
	if keyword == "" {
		return models.SearchTask{}, false, nil
	}

	priceStart, err := parsePriceCell(cellAt(row, colPriceStart), defaultPriceStart)
	if err != nil {
		return models.SearchTask{}, false, fmt.Errorf("price_start: %w", err)
	}
	priceEnd, err := parsePriceCell(cellAt(row, colPriceEnd), defaultPriceEnd)
	if err != nil {
		return models.SearchTask{}, false, fmt.Errorf("price_end: %w", err)
	}
	if priceStart > priceEnd {
		return models.SearchTask{}, false, fmt.Errorf("price_start (%d) 不可大於 price_end (%d)", priceStart, priceEnd)
	}

	return models.SearchTask{
		Keyword:    keyword,
		PriceStart: priceStart,
		PriceEnd:   priceEnd,
	}, true, nil
}

func cellAt(row []string, idx int) string {
	if idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func parsePriceCell(raw string, fallback int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}

	var b strings.Builder
	for _, r := range raw {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	digits := b.String()
	if digits == "" {
		return 0, fmt.Errorf("無法解析價格 %q", raw)
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0, fmt.Errorf("無法解析價格 %q", raw)
	}
	if n < 0 {
		return 0, fmt.Errorf("價格不可為負數: %d", n)
	}
	return n, nil
}

func taskDedupeKey(t models.SearchTask) string {
	return fmt.Sprintf("%s|%d|%d", t.Keyword, t.PriceStart, t.PriceEnd)
}
