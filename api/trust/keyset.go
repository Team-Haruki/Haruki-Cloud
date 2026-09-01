// Package trust serves the offline-signed trust keyset to bot clients.
//
// The keyset is produced out of band by cmd/trust-signer with the offline root
// key. The Cloud never holds that key; it only republishes the signed envelope
// verbatim, so a compromised Cloud cannot mint a keyset the client would trust.
package trust

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"haruki-cloud/api"
	"haruki-cloud/internal/core/trustsign"
	json "haruki-cloud/internal/jsonutil"

	"github.com/gofiber/fiber/v3"
)

// KeysetRoute is the public path clients fetch before authenticating.
const KeysetRoute = "/api/v3/trust/keyset"

// keysetReloadInterval bounds how often the file's mtime is re-checked so a
// republished keyset is picked up without a restart.
const keysetReloadInterval = 30 * time.Second

var errKeysetNotSigned = errors.New("trust keyset envelope is not a keyset signature")

// RegisterTrustRoutes mounts GET /api/v3/trust/keyset when keysetPath is set.
// The file must hold a trustsign.Envelope whose domain is DomainKeyset; its
// signature is not checked here because the root public key lives only in
// clients. Structural problems surface as a 503 with a warning log rather
// than serving something a client would reject anyway.
func RegisterTrustRoutes(app *fiber.App, keysetPath string) {
	keysetPath = strings.TrimSpace(keysetPath)
	if keysetPath == "" {
		return
	}
	source := &keysetSource{path: keysetPath, now: time.Now}
	app.Get(KeysetRoute, source.handler)
}

type keysetSource struct {
	path string
	now  func() time.Time

	mu        sync.Mutex
	checkedAt time.Time
	modTime   time.Time
	body      []byte
	err       error
}

func (s *keysetSource) handler(c fiber.Ctx) error {
	body, err := s.load()
	if err != nil {
		slog.WarnContext(c.Context(), "trust keyset unavailable", "error_type", fmt.Sprintf("%T", err))
		return api.JSONResponse(c, fiber.StatusServiceUnavailable, "信任密钥集暂不可用")
	}
	c.Set("Content-Type", "application/json; charset=utf-8")
	c.Set("Cache-Control", "public, max-age=60")
	return c.Status(fiber.StatusOK).Send(body)
}

// load returns the current envelope bytes, re-reading the file when its mtime
// changed and at most once per keysetReloadInterval.
func (s *keysetSource) load() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if s.body != nil && now.Sub(s.checkedAt) < keysetReloadInterval {
		return s.body, s.err
	}
	s.checkedAt = now

	info, err := os.Stat(s.path)
	if err != nil {
		s.body, s.err = nil, err
		return nil, err
	}
	if s.body != nil && info.ModTime().Equal(s.modTime) {
		return s.body, s.err
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		s.body, s.err = nil, err
		return nil, err
	}
	if err := validateKeysetEnvelope(raw); err != nil {
		s.body, s.err = nil, err
		return nil, err
	}
	s.body, s.err, s.modTime = raw, nil, info.ModTime()
	return raw, nil
}

func validateKeysetEnvelope(raw []byte) error {
	var env trustsign.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode trust keyset envelope: %w", err)
	}
	if env.Algorithm != trustsign.Algorithm || env.Domain != trustsign.DomainKeyset || len(env.Signature) == 0 || len(env.Payload) == 0 {
		return errKeysetNotSigned
	}
	var doc trustsign.KeysetDocument
	if err := json.Unmarshal(env.Payload, &doc); err != nil {
		return fmt.Errorf("decode trust keyset payload: %w", err)
	}
	return doc.Validate()
}
