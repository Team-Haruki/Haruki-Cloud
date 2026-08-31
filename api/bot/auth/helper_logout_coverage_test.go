package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

type deleteFailingRedisStore struct {
	*memoryRedisStore
	err error
}

func (s *deleteFailingRedisStore) Del(context.Context, string) error { return s.err }

func TestRedisKVStoreAndServiceHelperBranches(t *testing.T) {
	ctx := context.Background()
	nilStore := &redisKVStore{}
	if err := nilStore.Set(ctx, "k", "v", time.Second); !errors.Is(err, errRedisClientUnavailable) {
		t.Fatalf("nil Set error = %v", err)
	}
	if _, err := nilStore.Get(ctx, "k"); !errors.Is(err, errRedisClientUnavailable) {
		t.Fatalf("nil Get error = %v", err)
	}
	if err := nilStore.Del(ctx, "k"); !errors.Is(err, errRedisClientUnavailable) {
		t.Fatalf("nil Del error = %v", err)
	}
	if _, err := nilStore.Incr(ctx, "k"); !errors.Is(err, errRedisClientUnavailable) {
		t.Fatalf("nil Incr error = %v", err)
	}
	if err := nilStore.Expire(ctx, "k", time.Second); !errors.Is(err, errRedisClientUnavailable) {
		t.Fatalf("nil Expire error = %v", err)
	}
}

func TestRedisKVStoreOperations(t *testing.T) {
	ctx := context.Background()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := newRedisKVStore(client)
	if err := store.Set(ctx, "k", "v", time.Minute); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(ctx, "k"); err != nil || got != "v" {
		t.Fatalf("Get = %q,%v", got, err)
	}
	if got, err := store.Incr(ctx, "count"); err != nil || got != 1 {
		t.Fatalf("Incr = %d,%v", got, err)
	}
	if err := store.Expire(ctx, "count", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := store.Del(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "k"); !errors.Is(err, redis.Nil) {
		t.Fatalf("deleted Get error = %v", err)
	}
}

func TestServiceHelperBranches(t *testing.T) {
	ctx := context.Background()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	userService := NewUserService(nil, client, nil, "")
	internalService := NewInternalService(nil, client)
	if err := userService.setRedisKey(ctx, "test:%v", 7, "value", 1); err != nil {
		t.Fatal(err)
	}
	if got, err := userService.getRedisKey(ctx, "test:%v", 7); err != nil || got != "value" {
		t.Fatalf("user get helper = %q,%v", got, err)
	}
	if got, err := internalService.getRedisKey(ctx, "test:%v", 7); err != nil || got != "value" {
		t.Fatalf("internal get helper = %q,%v", got, err)
	}
	if err := userService.delRedisKey(ctx, "test:%v", 7); err != nil {
		t.Fatal(err)
	}
}

func TestServiceConstructorsAndBanCheckerBranches(t *testing.T) {
	ctx := context.Background()
	userService := NewUserServiceWithDependencies(nil, nil, []byte("key"), "noise")
	if userService.redisStore == nil {
		t.Fatal("nil user store was not defaulted")
	}
	internalService := NewInternalServiceWithStore(nil, nil)
	if internalService.redisStore == nil {
		t.Fatal("nil internal store was not defaulted")
	}
	if (*UserService)(nil).WithGlobalBanChecker(nil) != nil || (*InternalService)(nil).WithGlobalBanChecker(nil) != nil {
		t.Fatal("nil service ban checker should stay nil")
	}
	if banned, err := ownerIsGloballyBanned(ctx, nil, 1); err != nil || banned {
		t.Fatalf("nil ban checker = %v,%v", banned, err)
	}
	checker := &stubGlobalBanChecker{banned: true}
	if banned, err := ownerIsGloballyBanned(ctx, checker, 123); err != nil || !banned {
		t.Fatalf("ban checker = %v,%v", banned, err)
	}
	if NewUserHandler(userService).svc != userService || NewInternalHandler(internalService).svc != internalService || NewStatisticsHandler(NewStatisticsService(nil)).svc == nil {
		t.Fatal("handler constructors did not retain services")
	}
}

func TestLogoutHandlerBranches(t *testing.T) {
	store := newMemoryRedisStore()
	service := NewUserServiceWithDependencies(nil, store, nil, "")
	handler := NewUserHandler(service)
	app := fiber.New()
	app.Post("/logout/:bot_id", handler.Logout)

	request := func(botID, token string) testEnvelope {
		headers := map[string]string{}
		if token != "" {
			headers["X-Haruki-Bot-Session-Token"] = token
		}
		return sendJSONRequest(t, app, http.MethodPost, "/logout/"+botID, `{}`, headers)
	}
	invalidReq, err := http.NewRequest(http.MethodPost, "/logout/invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	invalidReq.Header.Set("X-Haruki-Bot-Session-Token", "token")
	invalidResp, err := app.Test(invalidReq)
	if err != nil {
		t.Fatal(err)
	}
	invalidResp.Body.Close()
	if invalidResp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("invalid bot id status = %d", invalidResp.StatusCode)
	}
	if got := request("42", ""); got.Status != fiber.StatusUnauthorized {
		t.Fatalf("missing token status = %d", got.Status)
	}
	if got := request("42", "wrong"); got.Status != fiber.StatusUnauthorized {
		t.Fatalf("unknown session status = %d", got.Status)
	}
	store.value[fmt.Sprintf(RedisKeySessionToken, "42")] = "right"
	if got := request("42", "wrong"); got.Status != fiber.StatusUnauthorized {
		t.Fatalf("mismatched session status = %d", got.Status)
	}
	if got := request("42", "right"); got.Status != fiber.StatusOK {
		t.Fatalf("logout status = %d message=%s", got.Status, got.Message)
	}
	if _, ok := store.value[fmt.Sprintf(RedisKeySessionToken, "42")]; ok {
		t.Fatal("logout did not delete session")
	}

	wantErr := errors.New("delete failed")
	failing := &deleteFailingRedisStore{memoryRedisStore: newMemoryRedisStore(), err: wantErr}
	failing.value[fmt.Sprintf(RedisKeySessionToken, "7")] = "token"
	failingApp := fiber.New()
	failingApp.Post("/logout/:bot_id", NewUserHandler(NewUserServiceWithDependencies(nil, failing, nil, "")).Logout)
	failingReq, err := http.NewRequest(http.MethodPost, "/logout/7", nil)
	if err != nil {
		t.Fatal(err)
	}
	failingReq.Header.Set("X-Haruki-Bot-Session-Token", "token")
	failingResp, err := failingApp.Test(failingReq)
	if err != nil {
		t.Fatal(err)
	}
	failingResp.Body.Close()
	if failingResp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("delete failure status = %d", failingResp.StatusCode)
	}
}

func TestIncrementStatisticCounterBranches(t *testing.T) {
	wantErr := errors.New("update failed")
	if err := incrementStatisticCounter(func() error { t.Fatal("create called"); return nil }, func() (int, error) { return 0, wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("update error = %v", err)
	}
	if err := incrementStatisticCounter(func() error { t.Fatal("create called"); return nil }, func() (int, error) { return 1, nil }); err != nil {
		t.Fatalf("updated counter error = %v", err)
	}
	created := false
	if err := incrementStatisticCounter(func() error { created = true; return nil }, func() (int, error) { return 0, nil }); err != nil || !created {
		t.Fatalf("counter create = %v created=%v", err, created)
	}
	createErr := errors.New("create failed")
	if err := incrementStatisticCounter(func() error { return createErr }, func() (int, error) { return 0, nil }); !errors.Is(err, createErr) {
		t.Fatalf("create error = %v", err)
	}
	if err := (*StatisticsHandler)(nil).updateRequestsRanking(context.Background(), 1); err != nil {
		t.Fatalf("nil statistics handler error = %v", err)
	}
	if err := (&StatisticsHandler{}).updateRequestsRanking(context.Background(), 1); err != nil {
		t.Fatalf("empty statistics handler error = %v", err)
	}
}
