package logging

import (
	"log/slog"
	"os"

	"product-monitor/shared/config"
)

// Setup configures the default slog logger: JSON to stdout, Warn/Error to Discord when configured.
func Setup(service string, cfg *config.Config) *slog.LevelVar {
	levelVar := &slog.LevelVar{}
	levelVar.Set(slog.LevelInfo)

	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: levelVar,
	})

	handler := slog.Handler(jsonHandler)
	alertURL := cfg.DiscordAlertWebhookURL
	if alertURL == "" && len(cfg.DiscordWebhookURLs) > 0 {
		alertURL = cfg.DiscordWebhookURLs[0]
	}
	if alertURL != "" {
		handler = newDiscordNotifyHandler(jsonHandler, alertURL, service)
	}

	slog.SetDefault(slog.New(handler))

	if cfg.AppMode == "dev" {
		levelVar.Set(slog.LevelDebug)
		slog.Debug("日誌層級已動態切換為: DEBUG 模式")
	}

	return levelVar
}
