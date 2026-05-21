package sheets

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const fetchTimeout = 30 * time.Second

// FetchKeywords 從 Google 試算表 CSV 匯出 URL 讀取關鍵字（免 API 金鑰）。
// column 可為欄位索引（0 起算）或標題列名稱（不分大小寫）。
func FetchKeywords(ctx context.Context, csvURL, column string) ([]string, error) {
	if csvURL == "" {
		return nil, fmt.Errorf("未設定 Google 試算表 CSV URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, csvURL, nil)
	if err != nil {
		return nil, fmt.Errorf("建立請求失敗: %w", err)
	}

	client := &http.Client{Timeout: fetchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下載試算表 CSV 失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("試算表 CSV 回應狀態碼: %d", resp.StatusCode)
	}

	return parseKeywordsCSV(resp.Body, column)
}

func parseKeywordsCSV(r io.Reader, column string) ([]string, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("解析 CSV 失敗: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("試算表 CSV 為空")
	}

	colIdx, err := resolveColumnIndex(records[0], column)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var out []string
	for i := 1; i < len(records); i++ {
		row := records[i]
		if colIdx >= len(row) {
			continue
		}
		kw := strings.TrimSpace(row[colIdx])
		if kw == "" {
			continue
		}
		if _, ok := seen[kw]; ok {
			continue
		}
		seen[kw] = struct{}{}
		out = append(out, kw)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("試算表中找不到有效關鍵字（欄位 %q）", column)
	}
	return out, nil
}

func resolveColumnIndex(header []string, column string) (int, error) {
	column = strings.TrimSpace(column)
	if column == "" {
		column = "0"
	}

	if idx, err := strconv.Atoi(column); err == nil {
		if idx < 0 {
			return 0, fmt.Errorf("欄位索引不可為負數: %d", idx)
		}
		return idx, nil
	}

	want := strings.ToLower(column)
	for i, h := range header {
		h = strings.TrimSpace(h)
		h = strings.TrimPrefix(h, "\ufeff") // UTF-8 BOM
		if strings.ToLower(h) == want {
			return i, nil
		}
	}
	// 標題列可能尚未設定，允許用常見名稱對第一欄
	if want == "keyword" || want == "keywords" || want == "關鍵字" {
		return 0, nil
	}
	return 0, fmt.Errorf("找不到欄位 %q，標題列: %v", column, header)
}

// BuildExportURL 由試算表 ID 與可選 gid 組出 CSV 匯出網址。
func BuildExportURL(spreadsheetID, gid string) string {
	id := strings.TrimSpace(spreadsheetID)
	if id == "" {
		return ""
	}
	u := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/export?format=csv", id)
	gid = strings.TrimSpace(gid)
	if gid != "" && gid != "0" {
		u += "&gid=" + gid
	}
	return u
}

// NormalizeSheetURL 接受完整匯出網址或一般試算表網址，轉成 CSV export URL。
func NormalizeSheetURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "/export?") && strings.Contains(raw, "format=csv") {
		return raw
	}
	// https://docs.google.com/spreadsheets/d/SPREADSHEET_ID/edit...
	const marker = "/d/"
	i := strings.Index(raw, marker)
	if i < 0 {
		return raw
	}
	rest := raw[i+len(marker):]
	end := len(rest)
	for _, sep := range []string{"/", "?", "#"} {
		if j := strings.IndexAny(rest, sep); j >= 0 && j < end {
			end = j
		}
	}
	id := strings.TrimSpace(rest[:end])
	if id == "" {
		return raw
	}
	gid := ""
	if j := strings.Index(raw, "gid="); j >= 0 {
		tail := raw[j+4:]
		for k, c := range tail {
			if !unicode.IsDigit(c) {
				gid = tail[:k]
				break
			}
		}
		if gid == "" {
			gid = tail
		}
	}
	return BuildExportURL(id, gid)
}
