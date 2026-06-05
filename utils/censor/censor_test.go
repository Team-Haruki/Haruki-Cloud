package censor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	censorenttest "haruki-cloud/database/censor/enttest"
	"haruki-cloud/database/censor/imagemodcache"
	"haruki-cloud/utils/logger"

	_ "github.com/mattn/go-sqlite3"
)

type fakeImageModerator struct {
	suggestion IMSSuggestion
	err        error
}

type fakeTextModerator struct {
	result map[string]any
	err    error
}

func (f fakeTextModerator) TextCensor(string) (map[string]any, error) {
	return f.result, f.err
}

func (f fakeImageModerator) ImageModerationURL(context.Context, string) (IMSSuggestion, error) {
	return f.suggestion, f.err
}

func newCensorTestService(t *testing.T, moderator ImageModerator) *Service {
	t.Helper()

	client := censorenttest.Open(t, "sqlite3", fmt.Sprintf("file:censor_image_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatalf("close censor client: %v", err)
		}
	})

	return &Service{
		Client:         client,
		ImageCensorAPI: moderator,
		Logger:         logger.NewLogger("CensorImageTest", "ERROR", io.Discard),
	}
}

func TestCensorImageRequestFailurePassesWithoutCache(t *testing.T) {
	ctx := context.Background()
	service := newCensorTestService(t, fakeImageModerator{err: errors.New("request timeout")})

	if ok := service.CensorImage(ctx, 123, "https://example.test/background.png"); !ok {
		t.Fatalf("CensorImage() = false, want true when moderation request fails")
	}

	count, err := service.Client.ImageModCache.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count image_mod_cache: %v", err)
	}
	if count != 0 {
		t.Fatalf("image_mod_cache count = %d, want 0", count)
	}
}

func TestCensorImageBlockIsCachedAndRejected(t *testing.T) {
	ctx := context.Background()
	imageURL := "https://example.test/blocked.png"
	service := newCensorTestService(t, fakeImageModerator{suggestion: IMSSuggestionBlock})

	if ok := service.CensorImage(ctx, 456, imageURL); ok {
		t.Fatalf("CensorImage() = true, want false for blocked moderation result")
	}

	cached, err := service.Client.ImageModCache.
		Query().
		Where(imagemodcache.URLEQ(imageURL)).
		Only(ctx)
	if err != nil {
		t.Fatalf("query image_mod_cache: %v", err)
	}
	if cached.Result != string(IMSSuggestionBlock) {
		t.Fatalf("cached result = %q, want %q", cached.Result, IMSSuggestionBlock)
	}
	if cached.HarukiUserID == nil || *cached.HarukiUserID != 456 {
		t.Fatalf("cached haruki_user_id = %v, want 456", cached.HarukiUserID)
	}
}

func TestCensorShortBioRequestFailureRejectsWithoutCache(t *testing.T) {
	ctx := context.Background()
	service := newCensorTestService(t, nil)
	service.TextCensorAPI = fakeTextModerator{err: errors.New("baidu text censor API error: code=17 msg=Open api daily request limit reached")}

	if ok := service.CensorShortBio(ctx, 789, "592703738580070400", "私は中国から来た学生ですよ！そして、奏も瑞希も大好きです！", "jp"); ok {
		t.Fatal("CensorShortBio() = true, want false when moderation request fails")
	}

	count, err := service.Client.ShortBio.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count short_bio: %v", err)
	}
	if count != 0 {
		t.Fatalf("short_bio count = %d, want 0", count)
	}
}
