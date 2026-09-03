// Package buildpolicy holds the client build policy the Cloud enforces at
// AuthV3 login: a release allowlist (which build_id / version pairs may log
// in, and when) plus emergency revocations by build, version, bot or source
// address.
//
// The build_id and version are self-reported by the client, so this is not
// remote attestation: a patched binary can lie. What the policy buys is that a
// re-distributed old build, or one whose identifier has been revoked, stops
// working the moment the operator publishes a new document, which raises the
// cost of a generic patch from "strip one check" to "forge a live identity".
// It must be paired with short sessions and anomaly detection, not used alone.
package buildpolicy

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

// Build describes one released client binary.
type Build struct {
	// BuildID is the identifier the client reports in AuthV3 (`build_id`).
	BuildID string `json:"build_id"`
	// Version is the client version that build_id was released as. A login
	// reporting the same build_id under a different version is rejected.
	Version string `json:"version"`
	// Target names the platform/arch the binary was built for (informational
	// unless the client reports `target`).
	Target string `json:"target,omitempty"`
	// SHA256 of the released binary (hex). Only checked when the client
	// reports `binary_sha256`; a self-reported hash is a tripwire, not proof.
	SHA256 string `json:"sha256,omitempty"`
	// NotBefore / NotAfter bound the login window (unix seconds, 0 = open).
	NotBefore int64 `json:"not_before,omitempty"`
	NotAfter  int64 `json:"not_after,omitempty"`
	// Revoked pulls the build immediately, including active sessions.
	Revoked bool   `json:"revoked,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// Document is the policy file. Every field except Version and Builds is
// optional.
type Document struct {
	// Version must increase on every publication (informational for the
	// Cloud; clients never see this document).
	Version int64 `json:"version"`
	// IssuedAt / ExpiresAt are unix seconds. An expired document is treated as
	// unavailable (fail-open with a warning), never as "reject everyone".
	IssuedAt  int64 `json:"issued_at,omitempty"`
	ExpiresAt int64 `json:"expires_at,omitempty"`
	// Builds is the allowlist.
	Builds []Build `json:"builds"`
	// RevokedVersions lists client versions that may no longer log in, exact
	// ("3.0.0") or prefix with a trailing "*" ("3.0.*").
	RevokedVersions []string `json:"revoked_versions,omitempty"`
	// RevokedBots lists bot ids whose credential is pulled outright.
	RevokedBots []string `json:"revoked_bots,omitempty"`
	// BlockedSources lists client IPs or CIDRs refused at login.
	BlockedSources []string `json:"blocked_sources,omitempty"`
}

var (
	ErrInvalidDocument = errors.New("buildpolicy: invalid document")
	ErrExpired         = errors.New("buildpolicy: document has expired")
)

// Validate checks structural invariants so a broken publication is caught by
// trust-signer and by the Cloud loader before it is trusted.
func (d *Document) Validate() error {
	if d == nil {
		return fmt.Errorf("%w: nil", ErrInvalidDocument)
	}
	if d.Version <= 0 {
		return fmt.Errorf("%w: version must be positive", ErrInvalidDocument)
	}
	if d.ExpiresAt != 0 && d.IssuedAt != 0 && d.ExpiresAt <= d.IssuedAt {
		return fmt.Errorf("%w: expires_at must be after issued_at", ErrInvalidDocument)
	}
	seen := make(map[string]struct{}, len(d.Builds))
	for i := range d.Builds {
		b := &d.Builds[i]
		b.BuildID = strings.TrimSpace(b.BuildID)
		b.Version = strings.TrimSpace(b.Version)
		b.Target = strings.TrimSpace(b.Target)
		b.SHA256 = strings.ToLower(strings.TrimSpace(b.SHA256))
		if b.BuildID == "" {
			return fmt.Errorf("%w: builds[%d].build_id is empty", ErrInvalidDocument, i)
		}
		if b.Version == "" {
			return fmt.Errorf("%w: builds[%d].version is empty", ErrInvalidDocument, i)
		}
		if _, dup := seen[b.BuildID]; dup {
			return fmt.Errorf("%w: duplicate build_id %q", ErrInvalidDocument, b.BuildID)
		}
		seen[b.BuildID] = struct{}{}
		if b.SHA256 != "" {
			if raw, err := hex.DecodeString(b.SHA256); err != nil || len(raw) != 32 {
				return fmt.Errorf("%w: builds[%d].sha256 must be 32 bytes hex", ErrInvalidDocument, i)
			}
		}
		if b.NotAfter != 0 && b.NotBefore != 0 && b.NotAfter <= b.NotBefore {
			return fmt.Errorf("%w: builds[%d] not_after must be after not_before", ErrInvalidDocument, i)
		}
	}
	for i, v := range d.RevokedVersions {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%w: revoked_versions[%d] is empty", ErrInvalidDocument, i)
		}
	}
	for i, id := range d.RevokedBots {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("%w: revoked_bots[%d] is empty", ErrInvalidDocument, i)
		}
	}
	for i, src := range d.BlockedSources {
		if _, err := parseSource(src); err != nil {
			return fmt.Errorf("%w: blocked_sources[%d]: %v", ErrInvalidDocument, i, err)
		}
	}
	return nil
}

// ExpiredAt reports whether the document is past its expires_at.
func (d *Document) ExpiredAt(now time.Time) bool {
	return d != nil && d.ExpiresAt != 0 && now.Unix() > d.ExpiresAt
}

func (d *Document) findBuild(buildID string) *Build {
	for i := range d.Builds {
		if d.Builds[i].BuildID == buildID {
			return &d.Builds[i]
		}
	}
	return nil
}

func (d *Document) versionRevoked(version string) bool {
	for _, pattern := range d.RevokedVersions {
		pattern = strings.TrimSpace(pattern)
		if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
			if strings.HasPrefix(version, prefix) {
				return true
			}
			continue
		}
		if pattern == version {
			return true
		}
	}
	return false
}

func (d *Document) botRevoked(botID string) bool {
	for _, id := range d.RevokedBots {
		if strings.TrimSpace(id) == botID {
			return true
		}
	}
	return false
}

func (d *Document) sourceBlocked(ip string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, src := range d.BlockedSources {
		prefix, err := parseSource(src)
		if err != nil {
			continue
		}
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func parseSource(raw string) (netip.Prefix, error) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "/") {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			return netip.Prefix{}, err
		}
		return netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()), nil
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Prefix{}, err
	}
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}
