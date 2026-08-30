package censor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	censorenttest "haruki-cloud/database/censor/enttest"
	"haruki-cloud/database/censor/imagemodcache"
	"haruki-cloud/internal/observability/commandtrace"
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

type contextRecordingTextModerator struct {
	key      any
	value    any
	observed bool
}

func (f *contextRecordingTextModerator) TextCensor(string) (map[string]any, error) {
	return nil, errors.New("legacy TextCensor called")
}

func (f *contextRecordingTextModerator) TextCensorContext(ctx context.Context, _ string) (map[string]any, error) {
	f.observed = ctx.Value(f.key) == f.value
	return map[string]any{"conclusion": string(ResultCompliant)}, nil
}

func operationCount(snapshot commandtrace.Snapshot, name string) int {
	for _, operation := range snapshot.Operations {
		if operation.Name == name {
			return operation.Count
		}
	}
	return 0
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
	ctx, trace := commandtrace.WithTrace(context.Background())
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
	snapshot := trace.Snapshot()
	if got := operationCount(snapshot, "censor.cache"); got != 1 {
		t.Fatalf("censor.cache count = %d, want 1", got)
	}
	if got := operationCount(snapshot, "censor.store"); got != 1 {
		t.Fatalf("censor.store count = %d, want 1", got)
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

func TestCensorBusinessErrorLogOnlyIncludesErrorType(t *testing.T) {
	ctx := context.Background()
	service := newCensorTestService(t, nil)
	const secret = "client_secret=do-not-log&access_token=do-not-log"
	service.TextCensorAPI = fakeTextModerator{err: errors.New(secret)}
	var output bytes.Buffer
	service.Logger = logger.NewLogger("CensorSecurityTest", "ERROR", &output)

	if ok := service.CensorShortBio(ctx, 789, "592703738580070400", "private short bio", "jp"); ok {
		t.Fatal("CensorShortBio() = true, want false when moderation request fails")
	}
	logLine := output.String()
	if !strings.Contains(logLine, "error_type=") {
		t.Fatalf("log does not contain error_type: %q", logLine)
	}
	for _, forbidden := range []string{secret, "do-not-log", "592703738580070400", "private short bio"} {
		if strings.Contains(logLine, forbidden) {
			t.Fatalf("log leaked %q: %q", forbidden, logLine)
		}
	}
}

func TestCensorShortBioForwardsRequestContext(t *testing.T) {
	type contextKey string
	const key contextKey = "request"
	const value = "profile-settings"

	ctx := context.WithValue(context.Background(), key, value)
	moderator := &contextRecordingTextModerator{key: key, value: value}
	service := newCensorTestService(t, nil)
	service.TextCensorAPI = moderator

	if ok := service.CensorShortBio(ctx, 789, "123456", "hello", "jp"); !ok {
		t.Fatal("CensorShortBio() = false, want true")
	}
	if !moderator.observed {
		t.Fatal("context-aware moderator did not observe the request context")
	}
}

func TestCensorNameStoresAuditAndUsesCachedDecision(t *testing.T) {
	service := newCensorTestService(t, nil)
	service.TextCensorAPI = fakeTextModerator{result: map[string]any{"conclusion": string(ResultCompliant)}}

	if !service.CensorName(context.Background(), 42, "10001", "Miku", "jp") {
		t.Fatal("compliant name was rejected")
	}
	if count, err := service.Client.Result.Query().Count(context.Background()); err != nil || count != 1 {
		t.Fatalf("result cache count = %d, err=%v", count, err)
	}
	if count, err := service.Client.NameLog.Query().Count(context.Background()); err != nil || count != 1 {
		t.Fatalf("name audit count = %d, err=%v", count, err)
	}

	service.TextCensorAPI = fakeTextModerator{err: errors.New("cache should avoid moderator")}
	if !service.CensorName(context.Background(), 42, "10001", "Miku", "jp") {
		t.Fatal("cached compliant name was rejected")
	}
	if !service.CensorName(context.Background(), 42, "10001", "", "jp") ||
		!service.CensorName(context.Background(), 42, "10001", "anything", "cn") {
		t.Fatal("name moderation bypass was rejected")
	}
}

func TestCensorNameRejectsAndCachesNonCompliantDecision(t *testing.T) {
	service := newCensorTestService(t, nil)
	service.TextCensorAPI = fakeTextModerator{result: map[string]any{"conclusion": string(ResultNonCompliant)}}

	if service.CensorName(context.Background(), 7, "10002", "blocked", "jp") {
		t.Fatal("non-compliant name was accepted")
	}
	service.TextCensorAPI = fakeTextModerator{err: errors.New("cache should avoid moderator")}
	if service.CensorName(context.Background(), 7, "10002", "blocked", "jp") {
		t.Fatal("cached non-compliant name was accepted")
	}
}

func TestCensorShortBioStoresAndUsesBothCachedDecisions(t *testing.T) {
	service := newCensorTestService(t, nil)
	service.TextCensorAPI = fakeTextModerator{result: map[string]any{"conclusion": string(ResultCompliant)}}

	if !service.CensorShortBio(context.Background(), 8, "10003", "hello", "jp") {
		t.Fatal("compliant short bio was rejected")
	}
	service.TextCensorAPI = fakeTextModerator{result: map[string]any{"conclusion": string(ResultNonCompliant)}}
	if service.CensorShortBio(context.Background(), 8, "10003", "blocked bio", "jp") {
		t.Fatal("non-compliant short bio was accepted")
	}
	service.TextCensorAPI = fakeTextModerator{err: errors.New("cache should avoid moderator")}
	if !service.CensorShortBio(context.Background(), 8, "10003", "hello", "jp") {
		t.Fatal("cached compliant short bio was rejected")
	}
	if service.CensorShortBio(context.Background(), 8, "10003", "blocked bio", "jp") {
		t.Fatal("cached non-compliant short bio was accepted")
	}
	if !service.CensorShortBio(context.Background(), 8, "10003", "", "jp") ||
		!service.CensorShortBio(context.Background(), 8, "10003", "anything", "cn") {
		t.Fatal("short bio moderation bypass was rejected")
	}
}

func TestCensorImagePassesCachesAndSupportsDisabledModerator(t *testing.T) {
	disabled := newCensorTestService(t, nil)
	if !disabled.CensorImage(context.Background(), 0, "https://example.test/disabled.png") {
		t.Fatal("disabled image moderator rejected image")
	}

	service := newCensorTestService(t, fakeImageModerator{suggestion: IMSSuggestionPass})
	imageURL := "https://example.test/passed.png"
	if !service.CensorImage(context.Background(), 0, imageURL) {
		t.Fatal("passing image was rejected")
	}
	service.ImageCensorAPI = fakeImageModerator{err: errors.New("cache should avoid moderator")}
	if !service.CensorImage(context.Background(), 0, imageURL) {
		t.Fatal("cached passing image was rejected")
	}
}
