package jev

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
)

func (c *Client) logEnabled(ctx context.Context, level slog.Level) bool {
	if c.logger == nil || c.logLevel == "off" {
		return false
	}
	threshold := slog.LevelWarn
	switch c.logLevel {
	case "debug":
		threshold = slog.LevelDebug
	case "info":
		threshold = slog.LevelInfo
	case "error":
		threshold = slog.LevelError
	}
	return level >= threshold && c.logger.Enabled(ctx, level)
}
func (c *Client) log(ctx context.Context, level slog.Level, message string, args ...any) {
	if c.logEnabled(ctx, level) {
		c.logger.Log(ctx, level, message, args...)
	}
}
func (c *Client) logWire(ctx context.Context, direction, endpoint string, headers http.Header, body []byte) {
	if !c.logEnabled(ctx, slog.LevelDebug) {
		return
	}
	redacted := headers.Clone()
	for name := range redacted {
		lower := strings.ToLower(name)
		if lower == "authorization" || lower == "proxy-authorization" || lower == "x-api-key" || lower == "api-key" || lower == "cookie" || lower == "set-cookie" || strings.Contains(lower, "secret") || strings.Contains(lower, "token") {
			redacted[name] = []string{"[redacted]"}
		}
	}
	args := []any{"endpoint", endpoint, "headers", redacted}
	if c.logBodies {
		args = append(args, "body", string(body))
	}
	c.log(ctx, slog.LevelDebug, direction, args...)
}
