package engine

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"sync"

	playwright "github.com/playwright-community/playwright-go"
)

// 定義一個常見的真實瀏覽器 User-Agent 清單
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:125.0) Gecko/20100101 Firefox/125.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Safari/605.1.15",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0",
	"Mozilla/5.0 (X11; Linux x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
}

// GetRandomUserAgent 隨機取得一個真實瀏覽器的 User-Agent
func GetRandomUserAgent() string {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(userAgents))))
	if err != nil {
		return userAgents[0]
	}
	return userAgents[n.Int64()]
}

// IsRecoverableBrowserError 判斷是否為瀏覽器／Playwright 連線中斷類錯誤（可重啟後重試）。
func IsRecoverableBrowserError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	needles := []string{
		"pipe closed",
		"target closed",
		"target page, context or browser has been closed",
		"browser has been closed",
		"connection closed",
		"eof",
		"websocket",
		"protocol error",
		"session closed",
	}
	for _, n := range needles {
		if strings.Contains(msg, n) {
			return true
		}
	}
	return false
}

// BrowserManager 封裝 Playwright 實體，負責維護瀏覽器的生命週期與自癒重啟
type BrowserManager struct {
	mu       sync.Mutex
	PW       *playwright.Playwright
	Browser  playwright.Browser
	headless bool
}

// NewBrowserManager 建立並啟動瀏覽器管理器
func NewBrowserManager(headless bool) (*BrowserManager, error) {
	bm := &BrowserManager{headless: headless}
	if err := bm.relaunch(); err != nil {
		return nil, err
	}
	return bm, nil
}

// EnsureBrowserAlive 每次爬取任務前調用，確保瀏覽器在線
func (bm *BrowserManager) EnsureBrowserAlive() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.ensureAliveLocked()
}

// Recover 強制完整重啟 Playwright 驅動與 Chromium（連線 EOF 時使用）
func (bm *BrowserManager) Recover() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	slog.Warn("[Browser] 執行完整重啟 Playwright 與 Chromium...")
	return bm.relaunchLocked(true)
}

// NewContext 建立分頁上下文；若連線已死會自動重啟後重試一次
func (bm *BrowserManager) NewContext() (playwright.BrowserContext, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if err := bm.ensureAliveLocked(); err != nil {
		return nil, err
	}

	ctx, err := createContextLocked(bm.Browser)
	if err == nil {
		return ctx, nil
	}
	if !IsRecoverableBrowserError(err) {
		return nil, err
	}

	slog.Warn("[Browser] 建立分頁上下文失敗，嘗試完整重啟",
		"err_msg", err,
	)
	if relaunchErr := bm.relaunchLocked(true); relaunchErr != nil {
		return nil, fmt.Errorf("重啟後仍無法建立上下文: %w (原始: %v)", relaunchErr, err)
	}
	return createContextLocked(bm.Browser)
}

func (bm *BrowserManager) ensureAliveLocked() error {
	if bm.Browser != nil && bm.Browser.IsConnected() {
		return nil
	}
	slog.Warn("[Browser] 偵測到 Chromium 已斷線，正在重新啟動...")
	return bm.relaunchLocked(false)
}

// relaunch 公開包裝（啟動時使用）
func (bm *BrowserManager) relaunch() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.relaunchLocked(true)
}

// relaunchLocked 重啟瀏覽器；forcePW 為 true 時一併重啟 Playwright 驅動程序
func (bm *BrowserManager) relaunchLocked(forcePW bool) error {
	if bm.Browser != nil {
		_ = bm.Browser.Close()
		bm.Browser = nil
	}

	if forcePW && bm.PW != nil {
		_ = bm.PW.Stop()
		bm.PW = nil
	}

	if bm.PW == nil {
		pw, err := playwright.Run()
		if err != nil {
			return fmt.Errorf("啟動 Playwright 驅動失敗: %w", err)
		}
		bm.PW = pw
	}

	browser, err := InitBrowser(bm.PW, bm.headless)
	if err != nil {
		if !forcePW {
			// 僅重啟 browser 失敗時，再試一次連同 PW 一起重啟
			return bm.relaunchLocked(true)
		}
		return fmt.Errorf("建立 Chromium 失敗: %w", err)
	}

	bm.Browser = browser
	slog.Info("[Browser] Chromium 已就緒")
	return nil
}

// Close 關閉管理器並安全釋放所有 Playwright 資源
func (bm *BrowserManager) Close() {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	if bm.Browser != nil {
		_ = bm.Browser.Close()
		bm.Browser = nil
	}
	if bm.PW != nil {
		_ = bm.PW.Stop()
		bm.PW = nil
	}
}

// InitBrowser 初始化並啟動 Chromium 瀏覽器實體 (優化參數防 Docker OOM)
func InitBrowser(pw *playwright.Playwright, headless bool) (playwright.Browser, error) {
	if pw == nil {
		return nil, errors.New("playwright 驅動未初始化")
	}
	return pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
		Args: []string{
			"--disable-dev-shm-usage",
			"--disable-gpu",
			"--disable-blink-features=AutomationControlled",
			"--no-sandbox",
			"--js-flags=--max-old-space-size=512",
			"--disable-ipv6",
		},
	})
}

func createContextLocked(browser playwright.Browser) (playwright.BrowserContext, error) {
	if browser == nil {
		return nil, errors.New("browser 未初始化")
	}

	randomUA := GetRandomUserAgent()
	slog.Debug("[Browser] 正在建立新分頁環境")

	context, err := browser.NewContext(playwright.BrowserNewContextOptions{
		UserAgent: playwright.String(randomUA),
	})
	if err != nil {
		return nil, err
	}

	stealthJS := `Object.defineProperty(navigator, 'webdriver', {get: () => undefined})`
	_ = context.AddInitScript(playwright.Script{Content: &stealthJS})

	return context, nil
}
