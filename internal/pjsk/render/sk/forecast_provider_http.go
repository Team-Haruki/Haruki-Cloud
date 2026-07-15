package sk

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/version"

	json "github.com/bytedance/sonic"
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

func extractSekaRunAssignment(body string, name string) string {
	idx := strings.Index(body, name)
	if idx < 0 {
		return ""
	}
	remaining := body[idx+len(name):]
	eq := strings.IndexByte(remaining, '=')
	if eq < 0 {
		return ""
	}
	remaining = strings.TrimSpace(remaining[eq+1:])
	end := strings.IndexAny(remaining, ";\n\r")
	if end >= 0 {
		remaining = remaining[:end]
	}
	return strings.Trim(strings.TrimSpace(remaining), "\"'")
}

func extractSekaRunRows(body string) (string, []string, error) {
	currentEvent := extractSekaRunAssignment(body, "currentEvent")

	if start := strings.Index(body, "data = [["); start >= 0 {
		start += len("data = [[")
		end := strings.Index(body[start:], "]];")
		if end < 0 {
			return currentEvent, nil, fmt.Errorf("unexpected sekarun data payload")
		}
		payload := body[start : start+end]
		if strings.TrimSpace(payload) == "" {
			return currentEvent, nil, nil
		}
		return currentEvent, strings.Split(payload, "], ["), nil
	}

	start := strings.Index(body, "[[")
	end := strings.LastIndex(body, "]]")
	if start < 0 || end <= start+2 {
		return currentEvent, nil, fmt.Errorf("unexpected sekarun payload")
	}
	return currentEvent, strings.Split(body[start+2:end], "], ["), nil
}

func parseSekaRunScore(values []string) (int, bool) {
	if len(values) > 4 {
		score, err := strconv.ParseFloat(values[4], 64)
		if err == nil && score > 0 {
			return int(math.Round(score)), true
		}
	}
	if len(values) <= 9 {
		return 0, false
	}
	lower, errLower := strconv.ParseFloat(values[8], 64)
	upper, errUpper := strconv.ParseFloat(values[9], 64)
	if errLower != nil || errUpper != nil {
		return 0, false
	}
	score := int(math.Round((lower + upper) / 2))
	if score <= 0 {
		return 0, false
	}
	return score, true
}

func (p *RemoteForecastProvider) getJSON(ctx context.Context, url string, out any) error {
	finishHTTP := commandtrace.MeasureOperation(ctx, "forecast.http")
	resp, err := p.http.R().
		SetContext(ctx).
		SetHeader("User-Agent", version.UserAgent()).
		Get(url)
	finishHTTP()
	if err != nil {
		return err
	}
	if resp.StatusCode() != 200 {
		return fmt.Errorf("http %d", resp.StatusCode())
	}
	finishDecode := commandtrace.MeasureOperation(ctx, "forecast.decode")
	err = json.Unmarshal(resp.Body(), out)
	finishDecode()
	if err != nil {
		return err
	}
	return nil
}

func (p *RemoteForecastProvider) getText(ctx context.Context, url string) (string, error) {
	finishHTTP := commandtrace.MeasureOperation(ctx, "forecast.http")
	resp, err := p.http.R().
		SetContext(ctx).
		SetHeader("User-Agent", version.UserAgent()).
		Get(url)
	finishHTTP()
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
		return ts
	}
	return ts * 1000
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
	return parsed.UnixMilli(), true
}
