package parser

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	playwright "github.com/playwright-community/playwright-go"
	"product-monitor/shared/models"
)

func ParseListings(page playwright.Page) ([]models.Product, error) {
	// 稍微等待確保動態內容與廣告加載完成
	time.Sleep(3 * time.Second)

	// 定位所有商品卡片容器
	items, err := page.QuerySelectorAll("[data-testid^='listing-card-']")
	if err != nil {
		return nil, fmt.Errorf("無法取得商品列表: %v", err)
	}

	var products []models.Product

	for _, item := range items {
		// --- 廣告過濾邏輯 ---
		// 檢查父層或自身是否包含廣告特徵
		parent, _ := item.QuerySelector("xpath=..") // 往上一層找
		if parent != nil {
			idAttr, _ := parent.GetAttribute("id")
			if strings.Contains(idAttr, "ad-") {
				continue // 跳過廣告
			}
		}

		// --- 賣家名稱抓取 ---
		sellerName := "未知賣家"
		sellerEl, _ := item.QuerySelector("[data-testid='listing-card-text-seller-name']")
		if sellerEl != nil {
			sellerName, _ = sellerEl.InnerText()
		}

		// --- 標題抓取 (優先抓取圖片的 alt，最完整) ---
		title := "無標題"
		imageURL := ""
		imgEl, _ := item.QuerySelector("a[href^='/p/'] img")
		if imgEl != nil {
			// 優先嘗試從 img 的 alt 屬性拿最完整的商品標題
			altText, _ := imgEl.GetAttribute("alt")
			if altText != "" {
				title = altText
			}

			// 獲取商品圖片的原始 CDN 網址
			src, _ := imgEl.GetAttribute("src")
			if src != "" {
				imageURL = src
			}
		}

		// 備案：如果圖片沒 alt 或是解析有異常，再用 p 標籤覆寫標題
		if title == "無標題" {
			titleEl, _ := item.QuerySelector("p[style*='max-line']")
			if titleEl != nil {
				title, _ = titleEl.InnerText()
			}
		}

		// --- 價格抓取 ---
		price := "面議"
		// 尋找包含 NT$ 符號的價格標籤
		priceEl, _ := item.QuerySelector("p:has-text('NT$')")
		if priceEl != nil {
			price, _ = priceEl.InnerText()
		}

		// --- 連結與 ID 抓取 ---
		link := ""
		id := ""
		linkEl, _ := item.QuerySelector("a[href^='/p/']")
		if linkEl != nil {
			href, _ := linkEl.GetAttribute("href")
			if parsedURL, err := url.Parse(href); err == nil {
				parsedURL.RawQuery = "" // 拔除 ?t-id=... 等所有參數
				parsedURL.Fragment = "" // 拔除 # 後面的錨點

				// 組合出絕對路徑，並順手去掉結尾可能殘留的問號
				cleanPath := strings.TrimSuffix(parsedURL.String(), "?")
				link = "https://tw.carousell.com" + cleanPath
			} else {
				// 萬一解析失敗的極端防呆
				link = "https://tw.carousell.com" + href
			}

			// 從 data-testid 直接拿 ID 最準確
			testID, _ := item.GetAttribute("data-testid")
			parts := strings.Split(testID, "-")
			id = parts[len(parts)-1]
		}

		// 只有當 ID 和標題都存在時才加入列表
		if id != "" && title != "無標題" {
			products = append(products, models.Product{
				ID:         id,
				Title:      title,
				Price:      price,
				Link:       link,
				ImageURL:   imageURL,
				SellerName: sellerName,
			})
		}
	}

	return products, nil
}

// GetSearchURL 封裝搜尋網址的建構邏輯
func GetSearchURL(query string) string {
	return fmt.Sprintf(
		"https://tw.carousell.com/search/%s?addRecent=true&canChangeKeyword=true&includeSuggestions=true&price_start=0&price_end=95000&sort_by=3&t-search_query_source=direct_search",
		query,
	)
}
