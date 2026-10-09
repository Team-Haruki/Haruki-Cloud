// Package upstreamerr classifies failures of the services Haruki-Cloud calls:
// the Toolbox, the game data service (SekaiAPI), the ranking service
// (tracker), the deck service and the renderer (Drawing).
//
// The clients describe their errors with a Service, an HTTP status and the
// upstream message (Described), or classify them at the source (Kinded:
// sentinels, transport failures, missing configuration). Classify is the only
// place that interprets upstream messages; every message and status it relies
// on is listed in contract.go and pinned by the contract tests. UserError
// turns a classified failure into a typed user error whose text comes from
// the catalog (upstream.toml); the upstream text itself only reaches logs.
//
// This package imports no client: the clients import it.
package upstreamerr

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"

	"haruki-cloud/utils/usererror"
)

// Service names an upstream service.
type Service string

const (
	ServiceToolbox  Service = "toolbox"
	ServiceGameData Service = "game_data"
	ServiceRanking  Service = "ranking"
	ServiceDeck     Service = "deck"
	ServiceRender   Service = "render"
)

// Kind is what went wrong, independent of the service's wording.
type Kind string

const (
	KindUnknown Kind = ""
	// KindNotConfigured: the client is not wired (nil client, empty base URL).
	KindNotConfigured Kind = "not_configured"
	// KindAuth: the service rejected Cloud's credentials.
	KindAuth Kind = "auth"
	// KindIncompatible: the service does not speak the protocol Cloud uses
	// (outdated service, endpoint not deployed, unsupported content type).
	KindIncompatible Kind = "incompatible"
	// KindUnavailable: refused, unreachable, closed connection, 502/503/504,
	// open circuit breaker.
	KindUnavailable Kind = "unavailable"
	// KindTimeout: no answer in time.
	KindTimeout Kind = "timeout"
	// KindBadResponse: an answer Cloud could not use (5xx, undecodable body).
	KindBadResponse Kind = "bad_response"
	// KindRejected: the service rejected Cloud's request (other 4xx).
	KindRejected Kind = "rejected"
	// KindRateLimited: too many requests.
	KindRateLimited Kind = "rate_limited"
	// KindMaintenance: the game server is under maintenance.
	KindMaintenance Kind = "maintenance"
	// KindNotFound: a generic 404 without a more specific kind.
	KindNotFound Kind = "not_found"

	// Toolbox.
	KindAccountNotBound Kind = "account_not_bound"
	KindDataNotUploaded Kind = "data_not_uploaded"
	KindAccessDenied    Kind = "access_denied"
	KindOwnerBanned     Kind = "owner_banned"

	// Game data service.
	KindPlayerNotFound Kind = "player_not_found"

	// Ranking service.
	KindRankingNotFound   Kind = "ranking_not_found"
	KindNoRankingData     Kind = "no_ranking_data"
	KindRegionUnsupported Kind = "region_unsupported"

	// Renderer.
	KindDataInsufficient Kind = "data_insufficient"
	KindContentTooLarge  Kind = "content_too_large"
	KindAssetMissing     Kind = "asset_missing"
	KindAssetBroken      Kind = "asset_broken"
	KindAssetDownload    Kind = "asset_download"

	// Deck service.
	KindDataNotSynced   Kind = "data_not_synced"
	KindCacheExpired    Kind = "cache_expired"
	KindUserDataInvalid Kind = "user_data_invalid"
	KindEmptyResult     Kind = "empty_result"
)

// Described is an upstream error that carries the service, the HTTP status
// (0 when there was no response) and the upstream message (may be empty).
// Classify derives the Kind from these.
type Described interface {
	error
	UpstreamService() Service
	UpstreamStatus() int
	UpstreamMessage() string
}

// Kinded is an upstream error classified where it was created.
type Kinded interface {
	error
	UpstreamService() Service
	UpstreamKind() Kind
}

// Class is the result of Classify.
type Class struct {
	Service Service
	Kind    Kind
	Status  int
}

// Sentinel is a comparable upstream error value (use errors.Is) that is
// already classified.
type Sentinel struct {
	service Service
	kind    Kind
	status  int
	text    string
}

// NewSentinel builds a sentinel error. text is the Error() text, for logs.
func NewSentinel(service Service, kind Kind, status int, text string) *Sentinel {
	return &Sentinel{service: service, kind: kind, status: status, text: text}
}

func (s *Sentinel) Error() string            { return s.text }
func (s *Sentinel) UpstreamService() Service { return s.service }
func (s *Sentinel) UpstreamKind() Kind       { return s.kind }
func (s *Sentinel) UpstreamStatus() int      { return s.status }
func (s *Sentinel) UpstreamMessage() string  { return "" }

// TransportError is a request that got no HTTP answer. Its text is safe to
// log (no URL); Unwrap returns the original error so errors.Is keeps working.
type TransportError struct {
	service Service
	kind    Kind
	text    string
	err     error
}

// Transport wraps a transport failure of service. text replaces the
// original error text (which may contain internal URLs); when empty, the
// original text is kept.
func Transport(service Service, text string, err error) error {
	if err == nil {
		return nil
	}
	if text == "" {
		text = err.Error()
	}
	return &TransportError{service: service, kind: transportKind(err), text: text, err: err}
}

func (e *TransportError) Error() string            { return e.text }
func (e *TransportError) Unwrap() error            { return e.err }
func (e *TransportError) UpstreamService() Service { return e.service }
func (e *TransportError) UpstreamKind() Kind       { return e.kind }

// Tagged is a failure of service with a known kind and a cause.
type Tagged struct {
	service Service
	kind    Kind
	text    string
	err     error
}

// Tag marks err as a failure of service with kind. text is the Error()
// text; when empty, err's text is used.
func Tag(service Service, kind Kind, text string, err error) error {
	if text == "" && err != nil {
		text = err.Error()
	}
	return &Tagged{service: service, kind: kind, text: text, err: err}
}

func (e *Tagged) Error() string            { return e.text }
func (e *Tagged) Unwrap() error            { return e.err }
func (e *Tagged) UpstreamService() Service { return e.service }
func (e *Tagged) UpstreamKind() Kind       { return e.kind }

// transportKind tells a timeout from an unreachable service using only the
// error types of the standard library.
func transportKind(err error) Kind {
	if errors.Is(err, context.DeadlineExceeded) {
		return KindTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return KindTimeout
	}
	return KindUnavailable
}

// IsNetworkFailure reports whether err is a transport-level failure (refused,
// reset, DNS, closed connection, timeout) judged by its type.
func IsNetworkFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}

// Classify reports the service and kind of an upstream failure. ok is false
// when err does not come from an upstream service.
func Classify(err error) (Class, bool) {
	if err == nil {
		return Class{}, false
	}
	if kinded, ok := errors.AsType[Kinded](err); ok {
		class := Class{Service: kinded.UpstreamService(), Kind: kinded.UpstreamKind()}
		if described, ok := kinded.(Described); ok {
			class.Status = described.UpstreamStatus()
		}
		return class, true
	}
	if described, ok := errors.AsType[Described](err); ok {
		service := described.UpstreamService()
		status := described.UpstreamStatus()
		return Class{Service: service, Kind: classifyMessage(service, status, described.UpstreamMessage()), Status: status}, true
	}
	return Class{}, false
}

// Is reports whether err is an upstream failure of kind.
func Is(err error, kind Kind) bool {
	class, ok := Classify(err)
	return ok && class.Kind == kind
}

// IsService reports whether err is an upstream failure of service.
func IsService(err error, service Service) bool {
	class, ok := Classify(err)
	return ok && class.Service == service
}

// UserError turns an upstream failure into a typed user error. It returns
// nil when err is not an upstream failure, and err itself (unchanged) when
// it already is a typed user error.
func UserError(err error) *usererror.Error {
	if typed, ok := usererror.As(err); ok {
		return typed
	}
	class, ok := Classify(err)
	if !ok {
		return nil
	}
	return userErrorFor(class, err)
}
