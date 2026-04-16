package vlive

import (
	"context"
	"strings"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
)

type vliveContextKey string

type fakeSource struct {
	defaultRegion renderregion.Value
	lives         map[renderregion.Value][]*Live
	ctx           context.Context
	wantKey       vliveContextKey
	wantValue     string
}

func (f *fakeSource) DefaultRegion() renderregion.Value {
	return f.defaultRegion
}

func (f *fakeSource) GetLives(region renderregion.Value) ([]*Live, error) {
	if f.wantValue != "" {
		if f.ctx == nil {
			return nil, context.Canceled
		}
		value, _ := f.ctx.Value(f.wantKey).(string)
		if value != f.wantValue {
			return nil, context.Canceled
		}
	}
	return f.lives[region], nil
}

func (f *fakeSource) WithContext(ctx context.Context) DataSource {
	clone := *f
	clone.ctx = ctx
	return &clone
}

func TestRenderTextFiltersAndFormatsLives(t *testing.T) {
	now := time.Date(2026, 3, 26, 20, 0, 0, 0, time.Local)
	ms := func(tm time.Time) int64 { return tm.UnixMilli() }

	controller := NewController(&fakeSource{
		defaultRegion: renderregion.JP,
		lives: map[renderregion.Value][]*Live{
			renderregion.JP: {
				{
					ID:      1001,
					Name:    "Future Live",
					StartAt: ms(now.Add(2 * time.Hour)),
					EndAt:   ms(now.Add(4 * time.Hour)),
					Schedules: []Schedule{
						{StartAt: ms(now.Add(2 * time.Hour)), EndAt: ms(now.Add(3 * time.Hour))},
						{StartAt: ms(now.Add(3*time.Hour + 30*time.Minute)), EndAt: ms(now.Add(4 * time.Hour))},
					},
				},
				{
					ID:      1002,
					Name:    "Ongoing Live",
					StartAt: ms(now.Add(-1 * time.Hour)),
					EndAt:   ms(now.Add(2 * time.Hour)),
					Schedules: []Schedule{
						{StartAt: ms(now.Add(-30 * time.Minute)), EndAt: ms(now.Add(30 * time.Minute))},
					},
				},
				{
					ID:      1003,
					Name:    "Too Far",
					StartAt: ms(now.Add(8 * 24 * time.Hour)),
					EndAt:   ms(now.Add(8*24*time.Hour + time.Hour)),
				},
				{
					ID:      1004,
					Name:    "Already Ended",
					StartAt: ms(now.Add(-2 * time.Hour)),
					EndAt:   ms(now.Add(-1 * time.Hour)),
				},
				{
					ID:      1005,
					Name:    "Too Long",
					StartAt: ms(now.Add(time.Hour)),
					EndAt:   ms(now.Add(31 * 24 * time.Hour)),
				},
			},
		},
	}, renderregion.JP)

	text, err := controller.RenderText(ListQuery{Now: now})
	if err != nil {
		t.Fatalf("RenderText() error = %v", err)
	}
	if !strings.Contains(text, "JP 虚拟Live列表") {
		t.Fatalf("missing header: %q", text)
	}
	if !strings.Contains(text, "【1001】Future Live") || !strings.Contains(text, "下一场:") {
		t.Fatalf("missing future live text: %q", text)
	}
	if !strings.Contains(text, "【1002】Ongoing Live") || !strings.Contains(text, "当前Live进行中") {
		t.Fatalf("missing ongoing live text: %q", text)
	}
	if !strings.Contains(text, "剩余场次: 2") {
		t.Fatalf("missing rest count: %q", text)
	}
	if strings.Contains(text, "Too Far") || strings.Contains(text, "Already Ended") || strings.Contains(text, "Too Long") {
		t.Fatalf("unexpected filtered lives in text: %q", text)
	}
}

func TestRenderTextReturnsEmptyMessageWhenNoUpcomingLives(t *testing.T) {
	controller := NewController(&fakeSource{
		defaultRegion: renderregion.JP,
		lives:         map[renderregion.Value][]*Live{renderregion.JP: nil},
	}, renderregion.JP)

	text, err := controller.RenderText(ListQuery{
		Now: time.Date(2026, 3, 26, 20, 0, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatalf("RenderText() error = %v", err)
	}
	if text != "当前没有虚拟Live" {
		t.Fatalf("unexpected empty text: %q", text)
	}
}

func TestControllerWithContextClonesVLiveSource(t *testing.T) {
	now := time.Date(2026, 3, 26, 20, 0, 0, 0, time.Local)
	ms := func(tm time.Time) int64 { return tm.UnixMilli() }

	controller := NewController(&fakeSource{
		defaultRegion: renderregion.JP,
		wantKey:       vliveContextKey("trace"),
		wantValue:     "vlive-list",
		lives: map[renderregion.Value][]*Live{
			renderregion.JP: {
				{ID: 1, Name: "Ctx Live", StartAt: ms(now.Add(time.Hour)), EndAt: ms(now.Add(2 * time.Hour))},
			},
		},
	}, renderregion.JP)

	ctx := context.WithValue(context.Background(), vliveContextKey("trace"), "vlive-list")
	text, err := controller.WithContext(ctx).RenderText(ListQuery{Now: now})
	if err != nil {
		t.Fatalf("RenderText() error = %v", err)
	}
	if !strings.Contains(text, "Ctx Live") {
		t.Fatalf("unexpected vlive text: %q", text)
	}
}
