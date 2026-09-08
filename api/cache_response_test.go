package api

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"haruki-cloud/config"
	json "haruki-cloud/internal/jsonutil"
)

type countedCachePayload struct{ calls *int }

func (p countedCachePayload) MarshalJSON() ([]byte, error) {
	*p.calls++
	return []byte(`{"id":9007199254740993,"title":"歌曲"}`), nil
}

func TestCachedResponseEncodedOnceAndReplayedExactly(t *testing.T) {
	original := config.Cfg
	config.Cfg.Backend.APICacheTTL = time.Minute
	t.Cleanup(func() { config.Cfg = original })
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	calls, fetches := 0, 0
	app := fiber.New(fiber.Config{JSONEncoder: json.Marshal, JSONDecoder: json.Unmarshal})
	app.Get("/cached", func(c fiber.Ctx) error {
		return WithCache(c, client, "wire", func(string) (any, error) { fetches++; return countedCachePayload{calls: &calls}, nil })
	})
	var first []byte
	for i := range 2 {
		response, err := app.Test(httptest.NewRequest("GET", "/cached", nil))
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 200 || response.Header.Get("Content-Type") != ContentTypeJSON {
			t.Fatalf("response=%d %v", response.StatusCode, response.Header)
		}
		if i == 0 {
			first = raw
		} else if string(raw) != string(first) {
			t.Fatalf("cached bytes changed: %s versus %s", raw, first)
		}
	}
	if calls != 1 || fetches != 1 {
		t.Fatalf("encodes=%d fetches=%d", calls, fetches)
	}
	cached, err := server.Get("wire:/cached:query=none")
	if err != nil {
		t.Fatal(err)
	}
	if cached != string(first) {
		t.Fatalf("cache differs from wire: %s", cached)
	}
	if ttl := server.TTL("wire:/cached:query=none"); ttl != time.Minute {
		t.Fatalf("TTL=%s", ttl)
	}
}

func TestCachedResponseRejectsInvalidEnvelope(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	app := fiber.New()
	app.Get("/cached", func(c fiber.Ctx) error {
		return WithCache(c, client, "invalid", func(string) (any, error) { t.Error("corrupt cache must not run fetch"); return nil, nil })
	})
	for _, raw := range []string{"", "{", "[]", "true", "123", `"text"`, `{} {}`} {
		t.Run(fmt.Sprintf("%q", raw), func(t *testing.T) {
			if err := client.Set(context.Background(), "invalid:/cached:query=none", raw, time.Minute).Err(); err != nil {
				t.Fatal(err)
			}
			response, err := app.Test(httptest.NewRequest("GET", "/cached", nil))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 500 {
				t.Fatalf("status=%d for %q", response.StatusCode, raw)
			}
		})
	}
}
