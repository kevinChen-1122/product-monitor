package logging

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"product-monitor/shared/discord"
)

// discordNotifyHandler writes to the wrapped handler, then forwards Warn/Error to Discord.
type discordNotifyHandler struct {
	next       slog.Handler
	webhookURL string
	service    string
}

func newDiscordNotifyHandler(next slog.Handler, webhookURL, service string) *discordNotifyHandler {
	return &discordNotifyHandler{
		next:       next,
		webhookURL: webhookURL,
		service:    service,
	}
}

func (h *discordNotifyHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *discordNotifyHandler) Handle(ctx context.Context, r slog.Record) error {
	if err := h.next.Handle(ctx, r); err != nil {
		return err
	}

	if r.Level >= slog.LevelWarn && h.webhookURL != "" {
		record := r
		go h.notify(record)
	}
	return nil
}

func (h *discordNotifyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &discordNotifyHandler{
		next:       h.next.WithAttrs(attrs),
		webhookURL: h.webhookURL,
		service:    h.service,
	}
}

func (h *discordNotifyHandler) WithGroup(name string) slog.Handler {
	return &discordNotifyHandler{
		next:       h.next.WithGroup(name),
		webhookURL: h.webhookURL,
		service:    h.service,
	}
}

func (h *discordNotifyHandler) notify(r slog.Record) {
	levelLabel := "WARN"
	color := 0xF39C12 // orange
	if r.Level >= slog.LevelError {
		levelLabel = "ERROR"
		color = 0xE74C3C // red
	}

	var fields []discord.Field
	r.Attrs(func(a slog.Attr) bool {
		fields = append(fields, discord.Field{
			Name:  a.Key,
			Value: truncate(attrString(a), 1024),
		})
		return true
	})

	loc, _ := time.LoadLocation("Asia/Taipei")
	timestamp := r.Time.In(loc).Format("2006-01-02 15:04:05")

	embed := discord.Embed{
		Title:       fmt.Sprintf("[%s] %s", h.service, levelLabel),
		Description: truncate(r.Message, 4096),
		Color:       color,
		Fields:      fields,
		Footer: &discord.Footer{
			Text: timestamp,
		},
	}

	// Avoid recursion: do not use slog if Discord delivery fails.
	_ = discord.SendEmbeds(h.webhookURL, []discord.Embed{embed})
}

func attrString(a slog.Attr) string {
	if a.Equal(slog.Attr{}) {
		return ""
	}
	if a.Value.Kind() == slog.KindString {
		return a.Value.String()
	}
	return fmt.Sprint(a.Value.Any())
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max-3]) + "..."
}
