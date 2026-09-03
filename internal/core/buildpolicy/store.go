package buildpolicy

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/core/trustsign"
)

// Mode selects what a failed evaluation does.
type Mode string

const (
	// ModeOff disables the policy entirely.
	ModeOff Mode = "off"
	// ModeLogOnly evaluates and reports but never rejects: the rollout mode
	// used to measure unlisted builds before switching to enforcement.
	ModeLogOnly Mode = "log-only"
	// ModeEnforce rejects logins and active sessions that fail the policy.
	ModeEnforce Mode = "enforce"
)

// ParseMode normalises the configured mode. An empty value means log-only when
// a policy path is configured and off otherwise.
func ParseMode(raw string, pathConfigured bool) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		if pathConfigured {
			return ModeLogOnly, nil
		}
		return ModeOff, nil
	case "off", "disabled":
		return ModeOff, nil
	case "log-only", "log_only", "logonly", "audit":
		return ModeLogOnly, nil
	case "enforce", "enforced", "on":
		return ModeEnforce, nil
	}
	return "", fmt.Errorf("buildpolicy: unknown mode %q (want off, log-only or enforce)", raw)
}

// Decision codes. CodeOK and CodePolicyUnavailable are the two allowed
// outcomes; everything else is a rejection (enforced or merely reported
// depending on the mode).
const (
	CodeOK                = "ok"
	CodePolicyOff         = "policy_off"
	CodePolicyUnavailable = "policy_unavailable"
	CodeBotRevoked        = "bot_revoked"
	CodeSourceBlocked     = "source_blocked"
	CodeVersionRevoked    = "version_revoked"
	CodeBuildMissing      = "build_missing"
	CodeBuildUnknown      = "build_unknown"
	CodeBuildRevoked      = "build_revoked"
	CodeBuildVersion      = "build_version_mismatch"
	CodeBuildNotYetValid  = "build_not_yet_valid"
	CodeBuildExpired      = "build_expired"
	CodeBuildTarget       = "build_target_mismatch"
	CodeBuildHash         = "build_hash_mismatch"
)

// Request is what a login (or an active session) reports.
type Request struct {
	BotID         string
	ClientVersion string
	BuildID       string
	Target        string
	BinarySHA256  string
	SourceIP      string
	Now           time.Time
}

// Decision is the evaluation outcome. Allowed already accounts for the mode:
// under log-only a failing request is Allowed with a non-OK Code so the
// caller can report it; under enforce it is not Allowed.
type Decision struct {
	Allowed bool
	// Passed is the raw policy verdict regardless of mode.
	Passed  bool
	Code    string
	Reason  string
	Enforce bool
}

// Store loads the policy file lazily and re-reads it when its mtime changes,
// at most once per reload interval. A nil *Store behaves as ModeOff.
type Store struct {
	path           string
	mode           Mode
	rootPub        ed25519.PublicKey
	now            func() time.Time
	reloadInterval time.Duration

	mu        sync.Mutex
	checkedAt time.Time
	modTime   time.Time
	doc       *Document
	err       error
}

const defaultReloadInterval = 30 * time.Second

// NewStore returns a store for path. rootPub, when non-nil, requires the file
// to be a trustsign envelope with DomainRelease verified by that key.
func NewStore(path string, mode Mode, rootPub ed25519.PublicKey) *Store {
	return &Store{
		path:           strings.TrimSpace(path),
		mode:           mode,
		rootPub:        rootPub,
		now:            time.Now,
		reloadInterval: defaultReloadInterval,
	}
}

// Mode reports the configured mode (ModeOff for a nil store).
func (s *Store) Mode() Mode {
	if s == nil {
		return ModeOff
	}
	return s.mode
}

// Enforcing reports whether failed evaluations are rejected.
func (s *Store) Enforcing() bool {
	return s.Mode() == ModeEnforce
}

// Document returns the current policy, reloading from disk when needed.
func (s *Store) Document() (*Document, error) {
	if s == nil || s.mode == ModeOff {
		return nil, errors.New("buildpolicy: policy is off")
	}
	if s.path == "" {
		return nil, errors.New("buildpolicy: no policy path configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if (s.doc != nil || s.err != nil) && now.Sub(s.checkedAt) < s.reloadInterval {
		return s.doc, s.err
	}
	s.checkedAt = now

	info, err := os.Stat(s.path)
	if err != nil {
		s.doc, s.err = nil, err
		return nil, err
	}
	if s.doc != nil && info.ModTime().Equal(s.modTime) {
		return s.doc, nil
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		s.doc, s.err = nil, err
		return nil, err
	}
	doc, err := Parse(raw, s.rootPub)
	if err != nil {
		s.doc, s.err = nil, err
		return nil, err
	}
	s.doc, s.err, s.modTime = doc, nil, info.ModTime()
	return doc, nil
}

// Parse decodes a policy file. It accepts a bare Document, or a trustsign
// envelope (domain DomainRelease) whose signature is verified against rootPub
// when rootPub is non-nil; with a root key configured a bare document is
// refused.
func Parse(raw []byte, rootPub ed25519.PublicKey) (*Document, error) {
	var env trustsign.Envelope
	payload := raw
	if err := json.Unmarshal(raw, &env); err == nil && env.Algorithm == trustsign.Algorithm && len(env.Signature) > 0 {
		if env.Domain != trustsign.DomainRelease {
			return nil, fmt.Errorf("buildpolicy: envelope domain %q is not %q", env.Domain, trustsign.DomainRelease)
		}
		if rootPub != nil {
			if err := trustsign.Verify(rootPub, env, trustsign.DomainRelease); err != nil {
				return nil, fmt.Errorf("buildpolicy: signature rejected: %w", err)
			}
		}
		payload = env.Payload
	} else if rootPub != nil {
		return nil, errors.New("buildpolicy: a signed envelope is required when a root public key is configured")
	}
	var doc Document
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, fmt.Errorf("buildpolicy: decode document: %w", err)
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return &doc, nil
}

// Evaluate applies the policy to a login attempt.
func (s *Store) Evaluate(req Request) Decision {
	if s == nil || s.mode == ModeOff {
		return Decision{Allowed: true, Passed: true, Code: CodePolicyOff}
	}
	enforce := s.mode == ModeEnforce
	doc, err := s.Document()
	if err != nil {
		return Decision{Allowed: true, Passed: false, Code: CodePolicyUnavailable, Reason: err.Error(), Enforce: enforce}
	}
	now := req.Now
	if now.IsZero() {
		now = s.now()
	}
	if doc.ExpiredAt(now) {
		return Decision{Allowed: true, Passed: false, Code: CodePolicyUnavailable, Reason: ErrExpired.Error(), Enforce: enforce}
	}
	code, reason := evaluateLogin(doc, req, now)
	passed := code == CodeOK
	return Decision{Allowed: passed || !enforce, Passed: passed, Code: code, Reason: reason, Enforce: enforce}
}

func evaluateLogin(doc *Document, req Request, now time.Time) (string, string) {
	if code, reason := doc.revocationFor(req); code != CodeOK {
		return code, reason
	}
	buildID := strings.TrimSpace(req.BuildID)
	if buildID == "" {
		return CodeBuildMissing, "client reported no build_id"
	}
	build := doc.findBuild(buildID)
	if build == nil {
		return CodeBuildUnknown, "build_id not in release allowlist"
	}
	return build.check(req, now)
}

// revocationFor applies the identity-level revocations (bot, source, version)
// that reject a login before the build entry is even looked up.
func (d *Document) revocationFor(req Request) (string, string) {
	if botID := strings.TrimSpace(req.BotID); botID != "" && d.botRevoked(botID) {
		return CodeBotRevoked, "bot credential revoked"
	}
	if req.SourceIP != "" && d.sourceBlocked(req.SourceIP) {
		return CodeSourceBlocked, "source address blocked"
	}
	if version := strings.TrimSpace(req.ClientVersion); version != "" && d.versionRevoked(version) {
		return CodeVersionRevoked, "client version revoked"
	}
	return CodeOK, ""
}

// check compares one allowlisted build entry with what the client reported.
func (b *Build) check(req Request, now time.Time) (string, string) {
	if b.Revoked {
		return CodeBuildRevoked, nonEmpty(b.Reason, "build revoked")
	}
	if strings.TrimSpace(req.ClientVersion) != b.Version {
		return CodeBuildVersion, fmt.Sprintf("build_id belongs to version %s", b.Version)
	}
	if b.NotBefore != 0 && now.Unix() < b.NotBefore {
		return CodeBuildNotYetValid, "build not yet valid"
	}
	if b.NotAfter != 0 && now.Unix() > b.NotAfter {
		return CodeBuildExpired, "build past its validity window"
	}
	if target := strings.TrimSpace(req.Target); target != "" && b.Target != "" && !strings.EqualFold(target, b.Target) {
		return CodeBuildTarget, fmt.Sprintf("build_id belongs to target %s", b.Target)
	}
	if hash := strings.ToLower(strings.TrimSpace(req.BinarySHA256)); hash != "" && b.SHA256 != "" && hash != b.SHA256 {
		return CodeBuildHash, "reported binary hash does not match the release"
	}
	return CodeOK, ""
}

// SessionAllowed re-checks an already issued session against the revocation
// parts of the policy only: bot, version and build revocations plus the
// build validity window. Unknown builds are not rejected here because a
// session that exists was already admitted under the mode in force at login.
func (s *Store) SessionAllowed(botID, clientVersion, buildID string, now time.Time) Decision {
	if s == nil || s.mode == ModeOff {
		return Decision{Allowed: true, Passed: true, Code: CodePolicyOff}
	}
	enforce := s.mode == ModeEnforce
	doc, err := s.Document()
	if err != nil {
		return Decision{Allowed: true, Passed: false, Code: CodePolicyUnavailable, Reason: err.Error(), Enforce: enforce}
	}
	if now.IsZero() {
		now = s.now()
	}
	if doc.ExpiredAt(now) {
		return Decision{Allowed: true, Passed: false, Code: CodePolicyUnavailable, Reason: ErrExpired.Error(), Enforce: enforce}
	}
	code, reason := CodeOK, ""
	switch {
	case strings.TrimSpace(botID) != "" && doc.botRevoked(strings.TrimSpace(botID)):
		code, reason = CodeBotRevoked, "bot credential revoked"
	case strings.TrimSpace(clientVersion) != "" && doc.versionRevoked(strings.TrimSpace(clientVersion)):
		code, reason = CodeVersionRevoked, "client version revoked"
	default:
		if build := doc.findBuild(strings.TrimSpace(buildID)); build != nil {
			switch {
			case build.Revoked:
				code, reason = CodeBuildRevoked, nonEmpty(build.Reason, "build revoked")
			case build.NotAfter != 0 && now.Unix() > build.NotAfter:
				code, reason = CodeBuildExpired, "build past its validity window"
			}
		}
	}
	passed := code == CodeOK
	return Decision{Allowed: passed || !enforce, Passed: passed, Code: code, Reason: reason, Enforce: enforce}
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
