package models

// SearchTask 為 Scheduler 派發至 Scraper 的搜尋任務。
type SearchTask struct {
	Keyword    string `json:"keyword"`
	PriceStart int    `json:"price_start"`
	PriceEnd   int    `json:"price_end"`
}
