package vlive

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type Controller struct {
	source        DataSource
	defaultRegion renderregion.Value
}

type contextualDataSource interface {
	WithContext(ctx context.Context) DataSource
}

func NewController(source DataSource, defaultRegion renderregion.Value) *Controller {
	return &Controller{
		source:        source,
		defaultRegion: renderregion.WithDefault(defaultRegion),
	}
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	if contextual, ok := any(c.source).(contextualDataSource); ok {
		clone.source = contextual.WithContext(ctx)
	}
	return &clone
}

func (c *Controller) ResolveLives(query ListQuery) ([]ResolvedLive, renderregion.Value, error) {
	if c == nil || c.source == nil {
		return nil, renderregion.Unknown, fmt.Errorf("vlive controller is not configured")
	}

	region := c.resolveRegion(query.Region)
	now := query.Now
	if now.IsZero() {
		now = time.Now()
	}

	lives, err := c.source.GetLives(region)
	if err != nil {
		return nil, region, err
	}

	result := make([]ResolvedLive, 0, len(lives))
	for _, live := range lives {
		if live == nil {
			continue
		}

		startAt := unixTime(live.StartAt)
		endAt := unixTime(live.EndAt)
		if startAt.IsZero() || endAt.IsZero() {
			continue
		}
		if !now.Before(endAt) {
			continue
		}
		if startAt.Sub(now) >= 7*24*time.Hour {
			continue
		}
		if endAt.Sub(startAt) >= 30*24*time.Hour {
			continue
		}

		schedules := normalizeSchedules(live.Schedules)
		resolved := ResolvedLive{
			ID:      live.ID,
			Name:    strings.TrimSpace(live.Name),
			StartAt: startAt,
			EndAt:   endAt,
		}
		for _, schedule := range schedules {
			if now.Before(schedule.EndAt) {
				current := schedule
				resolved.Current = &current
				resolved.Living = !now.Before(schedule.StartAt)
				break
			}
		}
		for _, schedule := range schedules {
			if now.Before(schedule.StartAt) {
				resolved.RestCount++
			}
		}
		if resolved.Current == nil {
			if now.Before(startAt) {
				resolved.Current = &Window{StartAt: startAt, EndAt: endAt}
			} else if now.Before(endAt) {
				resolved.Current = &Window{StartAt: startAt, EndAt: endAt}
				resolved.Living = true
			}
		}
		result = append(result, resolved)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].StartAt.Equal(result[j].StartAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].StartAt.Before(result[j].StartAt)
	})
	return result, region, nil
}

func (c *Controller) RenderText(query ListQuery) (string, error) {
	lives, region, err := c.ResolveLives(query)
	if err != nil {
		return "", err
	}
	if len(lives) == 0 {
		return "当前没有虚拟Live", nil
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("%s 虚拟Live列表", strings.ToUpper(region.String())))
	for _, live := range lives {
		builder.WriteString("\n\n")
		builder.WriteString(fmt.Sprintf("【%d】%s\n", live.ID, fallbackLiveName(live.Name, live.ID)))
		builder.WriteString(fmt.Sprintf("开始: %s\n", live.StartAt.Format("2006-01-02 15:04")))
		builder.WriteString(fmt.Sprintf("结束: %s\n", live.EndAt.Format("2006-01-02 15:04")))
		builder.WriteString("状态: ")
		switch {
		case live.Living:
			builder.WriteString("当前Live进行中")
		case live.Current != nil:
			builder.WriteString(fmt.Sprintf("下一场: %s", live.Current.StartAt.Format("2006-01-02 15:04")))
		default:
			builder.WriteString("已结束")
		}
		builder.WriteString(fmt.Sprintf(" | 剩余场次: %d", live.RestCount))
	}
	return builder.String(), nil
}

func (c *Controller) resolveRegion(region string) renderregion.Value {
	normalized := renderregion.Normalize(region)
	if !normalized.IsZero() {
		return normalized
	}
	if c.defaultRegion.IsZero() && c.source != nil {
		return renderregion.WithDefault(c.source.DefaultRegion())
	}
	return renderregion.WithDefault(c.defaultRegion)
}

func normalizeSchedules(items []Schedule) []Window {
	out := make([]Window, 0, len(items))
	for _, item := range items {
		startAt := unixTime(item.StartAt)
		endAt := unixTime(item.EndAt)
		if startAt.IsZero() || endAt.IsZero() || !startAt.Before(endAt) {
			continue
		}
		out = append(out, Window{StartAt: startAt, EndAt: endAt})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartAt.Equal(out[j].StartAt) {
			return out[i].EndAt.Before(out[j].EndAt)
		}
		return out[i].StartAt.Before(out[j].StartAt)
	})
	return out
}

func unixTime(value int64) time.Time {
	switch {
	case value <= 0:
		return time.Time{}
	case value < 1_000_000_000_000:
		return time.Unix(value, 0)
	default:
		return time.UnixMilli(value)
	}
}

func fallbackLiveName(name string, id int) string {
	if strings.TrimSpace(name) == "" {
		return fmt.Sprintf("Virtual Live #%d", id)
	}
	return name
}
