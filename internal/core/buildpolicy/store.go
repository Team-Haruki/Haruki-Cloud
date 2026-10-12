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

// Store loads the policy file lazily and re-reads it when its mtime (or the
// trust keyset's) changes, at most once per reload interval. A nil *Store
// behaves as ModeOff.
type Store struct {
	path           string
	keysetPath     string
	mode           Mode
	rootPub        ed25519.PublicKey
	now            func() time.Time
	reloadInterval time.Duration

	mu            sync.Mutex
	checkedAt     time.Time
	modTime       time.Time
	keysetModTime time.Time
	doc           *Document
	err           error
}

const defaultReloadInterval = 30 * time.Second

// NewStore returns a store for path. rootPub, when non-nil, requires the file
// to be a trustsign envelope with DomainRelease verified by that key, or by a
// release signing key listed in the root-signed keyset at keysetPath. Without
// rootPub nothing is verified and keysetPath is ignored.
func NewStore(path string, mode Mode, rootPub ed25519.PublicKey, keysetPath string) *Store {
	if rootPub == nil {
		keysetPath = ""
	}
	return &Store{
		path:           strings.TrimSpace(path),
		keysetPath:     strings.TrimSpace(keysetPath),
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

	doc, err := s.load(now)
	s.doc, s.err = doc, err
	return doc, err
}

// load re-reads the policy (and the keyset it may be verified against) when
// either file's mtime moved. It returns the cached document otherwise.
func (s *Store) load(now time.Time) (*Document, error) {
	info, err := os.Stat(s.path)
	if err != nil {
		return nil, err
	}
	var keysetModTime time.Time
	if s.keysetPath != "" {
		keysetInfo, err := os.Stat(s.keysetPath)
		if err != nil {
			return nil, fmt.Errorf("buildpolicy: trust keyset: %w", err)
		}
		keysetModTime = keysetInfo.ModTime()
	}
	if s.doc != nil && info.ModTime().Equal(s.modTime) && keysetModTime.Equal(s.keysetModTime) && !s.doc.signerLapsed(now) {
		return s.doc, nil
	}
	verifier, err := s.verifier(now)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	doc, err := Parse(raw, verifier)
	if err != nil {
		return nil, err
	}
	s.modTime, s.keysetModTime = info.ModTime(), keysetModTime
	return doc, nil
}

// verifier builds the key material for Parse: nil without a root key, the
// root alone without a keyset path, else the root plus the keyset the root
// signed (an unreadable or unverifiable keyset is a hard error so delegation
// never silently degrades to root-only).
func (s *Store) verifier(now time.Time) (*Verifier, error) {
	if s.rootPub == nil {
		return nil, nil
	}
	verifier := &Verifier{Root: s.rootPub, Now: now}
	if s.keysetPath == "" {
		return verifier, nil
	}
	raw, err := os.ReadFile(s.keysetPath)
	if err != nil {
		return nil, fmt.Errorf("buildpolicy: trust keyset: %w", err)
	}
	keyset, err := trustsign.ParseKeyset(raw, s.rootPub)
	if err != nil {
		return nil, fmt.Errorf("buildpolicy: trust keyset rejected: %w", err)
	}
	verifier.Keyset = keyset
	return verifier, nil
}

// Verifier holds what a signed policy may be checked against: the root public
// key, which signs policies directly, and optionally the root-verified keyset
// whose release_signing_keys are delegated for routine release signing.
type Verifier struct {
	Root   ed25519.PublicKey
	Keyset *trustsign.KeysetDocument
	// Now is the instant key validity windows are judged at; zero means
	// time.Now().
	Now time.Time
}

// verify checks the envelope's key_id against the keyset's release keys
// first; a listed key is bound to that id and never falls back to the root,
// so dropping a key from the keyset (or letting its window lapse) revokes it
// outright. An unlisted key_id must verify under the root key. For a
// delegated signer it returns the last unix second its validity covers (the
// earlier of the key's not_after and the keyset's expiry); 0 for the root.
func (v *Verifier) verify(env trustsign.Envelope) (int64, error) {
	now := v.Now
	if now.IsZero() {
		now = time.Now()
	}
	if v.Keyset != nil {
		if key, listed := v.Keyset.ReleaseKey(env.KeyID); listed {
			if v.Keyset.ExpiredAt(now) {
				return 0, fmt.Errorf("buildpolicy: release key %q: trust keyset has expired", env.KeyID)
			}
			if !key.ValidAt(now) {
				return 0, fmt.Errorf("buildpolicy: release key %q is outside its validity window", env.KeyID)
			}
			pub, err := trustsign.ParsePublicKeyHex(key.PublicKey)
			if err != nil {
				return 0, fmt.Errorf("buildpolicy: release key %q: %w", env.KeyID, err)
			}
			return min(key.NotAfter, v.Keyset.ExpiresAt-1), trustsign.Verify(pub, env, trustsign.DomainRelease)
		}
	}
	return 0, trustsign.Verify(v.Root, env, trustsign.DomainRelease)
}

// signerLapsed reports whether a delegated signer's validity has run out
// since the document was verified, so the cached copy must be re-checked
// (and rejected) even though neither file changed.
func (d *Document) signerLapsed(now time.Time) bool {
	return d.VerifiedUntil != 0 && now.Unix() > d.VerifiedUntil
}

// Parse decodes a policy file. It accepts a bare Document, or a trustsign
// envelope (domain DomainRelease) whose signature is verified through
// verifier when it is non-nil; with a verifier configured a bare document is
// refused. The loaded document records the key_id that signed it.
func Parse(raw []byte, verifier *Verifier) (*Document, error) {
	var env trustsign.Envelope
	payload := raw
	signedBy := ""
	var verifiedUntil int64
	if err := json.Unmarshal(raw, &env); err == nil && env.Algorithm == trustsign.Algorithm && len(env.Signature) > 0 {
		if env.Domain != trustsign.DomainRelease {
			return nil, fmt.Errorf("buildpolicy: envelope domain %q is not %q", env.Domain, trustsign.DomainRelease)
		}
		if verifier != nil {
			until, err := verifier.verify(env)
			if err != nil {
				return nil, fmt.Errorf("buildpolicy: signature rejected: %w", err)
			}
			verifiedUntil = until
		}
		payload, signedBy = env.Payload, env.KeyID
	} else if verifier != nil {
		return nil, errors.New("buildpolicy: a signed envelope is required when a root public key is configured")
	}
	var doc Document
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, fmt.Errorf("buildpolicy: decode document: %w", err)
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	doc.SignedBy, doc.VerifiedUntil = signedBy, verifiedUntil
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
