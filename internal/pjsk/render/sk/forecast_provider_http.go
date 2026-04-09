package sk

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func parseSekaRunRow(row string) []string {
	parts := strings.Split(row, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := strings.TrimSpace(part)
		clean = strings.Trim(clean, "[]\"'")
		out = append(out, clean)
	}
	return out
}

func (p *RemoteForecastProvider) getJSON(ctx context.Context, url string, out any) error {
	resp, err := p.http.R().
		SetContext(ctx).
		SetHeader("User-Agent", "HarukiCloud/1.0").
		Get(url)
	if err != nil {
		return err
	}
	if resp.StatusCode() != 200 {
		return fmt.Errorf("http %d", resp.StatusCode())
	}
	if err := json.Unmarshal(resp.Body(), out); err != nil {
		return err
	}
	return nil
}

func (p *RemoteForecastProvider) getText(ctx context.Context, url string) (string, error) {
	resp, err := p.http.R().
		SetContext(ctx).
		SetHeader("User-Agent", "HarukiCloud/1.0").
		Get(url)
	if err != nil {
		return "", err
	}
	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("http %d", resp.StatusCode())
	}
	return string(resp.Body()), nil
}

func asInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return 0, false
		}
		if i, err := strconv.Atoi(text); err == nil {
			return i, true
		}
		if f, err := strconv.ParseFloat(text, 64); err == nil {
			return int(f), true
		}
		return 0, false
	default:
		return 0, false
	}
}

func asInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return 0, false
		}
		if i, err := strconv.ParseInt(text, 10, 64); err == nil {
			return i, true
		}
		if f, err := strconv.ParseFloat(text, 64); err == nil {
			return int64(f), true
		}
		return 0, false
	default:
		return 0, false
	}
}

func normalizeForecastTimestamp(ts int64) int64 {
	if ts <= 0 {
		return 0
	}
	if ts > 1_000_000_000_000 {
		return ts / 1000
	}
	return ts
}

func parseForecastRFC3339(raw string) (int64, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return 0, false
	}
	return parsed.Unix(), true
}
