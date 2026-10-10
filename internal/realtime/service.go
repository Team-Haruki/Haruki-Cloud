package realtime

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"haruki-cloud/config"
	pjskdb "haruki-cloud/database/pjsk"
	harukiLogger "haruki-cloud/utils/logger"
)

var (
	// ErrInvalidSubscription means the stream credentials do not match an
	// active subscription version; the Client must stop reconnecting (401).
	ErrInvalidSubscription = errors.New("realtime: invalid subscription token")
	// ErrStaleSubscription means an ingested event targets a subscription
	// version that is no longer active (409).
	ErrStaleSubscription = errors.New("realtime: subscription is not active")
	// ErrReadOnly means this node cannot record deliveries (503).
	ErrReadOnly = errors.New("realtime: node is read-only")
)

// dbTimeout bounds one store call made outside a request context.
const dbTimeout = 5 * time.Second

// Options are the runtime dependencies of a Service.
type Options struct {
	Logger *harukiLogger.Logger
	// ReadOnly reports whether this node must not write (a standby); the
	// service then refuses streams and ingests with 503.
	ReadOnly func() bool
	// Now overrides the clock in tests.
	Now func() time.Time
}

// Service is the realtime event gateway: it accepts events, stores them,
// streams them to Clients and runs redelivery, stale-stream and GC sweeps.
type Service struct {
	cfg        config.EventsConfig
	store      *Store
	hub        *Hub
	log        *harukiLogger.Logger
	readOnly   func() bool
	now        func() time.Time
	ingestHash []byte
	forwarder  *forwarder
	stopping   atomic.Bool
}

// NewService builds a Service over the PJSK database client. cfg is
// normalised with WithDefaults.
func NewService(db *pjskdb.Client, cfg config.EventsConfig, opts Options) *Service {
	cfg = cfg.WithDefaults()
	s := &Service{
		cfg:      cfg,
		store:    NewStore(db),
		hub:      NewHub(cfg.MaxConns, cfg.MaxConnsPerSubscription),
		log:      opts.Logger,
		readOnly: opts.ReadOnly,
		now:      opts.Now,
	}
	if s.log == nil {
		s.log = harukiLogger.NewLoggerFromGlobal("Events")
	}
	if s.readOnly == nil {
		s.readOnly = func() bool { return false }
	}
	if s.now == nil {
		s.now = time.Now
	}
	if decoded, err := hex.DecodeString(cfg.IngestTokenSHA256); err == nil && len(decoded) == sha256.Size {
		s.ingestHash = decoded
	} else {
		s.log.Warn("realtime ingest token hash is missing or invalid; every ingest will be rejected",
			"event", "realtime_ingest_disabled")
	}
	if cfg.LegacyForwardURL != "" {
		s.forwarder = newForwarder(cfg.LegacyForwardURL, cfg.LegacyForwardToken, s.log)
	}
	return s
}

// Config returns the normalised configuration.
func (s *Service) Config() config.EventsConfig { return s.cfg }

// Hub exposes the live streams (close notifications, tests).
func (s *Service) Hub() *Hub { return s.hub }

// Store exposes the event store.
func (s *Service) Store() *Store { return s.store }

// Stopping reports whether shutdown has begun.
func (s *Service) Stopping() bool { return s.stopping.Load() }

// IngestAuthorized reports whether the Authorization header value carries
// the ingest token (bare or with a Bearer prefix).
func (s *Service) IngestAuthorized(header string) bool {
	if len(s.ingestHash) == 0 {
		return false
	}
	token := strings.TrimSpace(header)
	if len(token) >= len("bearer ") && strings.EqualFold(token[:len("bearer ")], "bearer ") {
		token = strings.TrimSpace(token[len("bearer "):])
	}
	if token == "" {
		return false
	}
	sum := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(sum[:], s.ingestHash) == 1
}

// tokenVersion is the version prefix of a subscription token
// ("<version>.<secret>").
func tokenVersion(token string) string {
	version, _, ok := strings.Cut(strings.TrimSpace(token), ".")
	if !ok {
		return ""
	}
	return strings.TrimSpace(version)
}

func subscriptionLive(sub *pjskdb.MysekaiBirthdaySubscription, version string, now time.Time) bool {
	return sub != nil && sub.Active && sub.ExpiresAt.After(now) && tokenVersion(sub.Token) == version
}

// Authenticate checks stream credentials against the subscription row.
// ErrInvalidSubscription is a permanent rejection; any other error is
// transient.
func (s *Service) Authenticate(ctx context.Context, rawSubscriptionID, version, token string) (Key, error) {
	id, err := strconv.Atoi(strings.TrimSpace(rawSubscriptionID))
	if err != nil || id <= 0 {
		return Key{}, ErrInvalidSubscription
	}
	version = strings.TrimSpace(version)
	sub, err := s.store.Subscription(ctx, id)
	if pjskdb.IsNotFound(err) {
		return Key{}, ErrInvalidSubscription
	}
	if err != nil {
		return Key{}, err
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(sub.Token)) != 1 ||
		!subscriptionLive(sub, version, s.now()) {
		return Key{}, ErrInvalidSubscription
	}
	return Key{SubscriptionID: id, Version: version}, nil
}

// IngestRequest is the body Toolbox posts to /internal/events.
type IngestRequest struct {
	EventID             string `json:"event_id"`
	SubscriptionID      string `json:"subscription_id"`
	SubscriptionVersion string `json:"subscription_version"`
	PayloadRef          string `json:"payload_ref"`
	EmptyResult         bool   `json:"empty_result"`
}

func (r *IngestRequest) normalize() {
	r.EventID = strings.TrimSpace(r.EventID)
	r.SubscriptionID = strings.TrimSpace(r.SubscriptionID)
	r.SubscriptionVersion = strings.TrimSpace(r.SubscriptionVersion)
	r.PayloadRef = strings.TrimSpace(r.PayloadRef)
}

// Ingest stores the event (idempotently), applies the replay policy and
// pushes it to live streams. created is false for a repeated notification.
func (s *Service) Ingest(ctx context.Context, req IngestRequest) (event Event, created bool, err error) {
	if s.readOnly() {
		return Event{}, false, ErrReadOnly
	}
	req.normalize()
	id, convErr := strconv.Atoi(req.SubscriptionID)
	if convErr != nil || id <= 0 {
		return Event{}, false, ErrStaleSubscription
	}
	sub, err := s.store.Subscription(ctx, id)
	if pjskdb.IsNotFound(err) {
		return Event{}, false, ErrStaleSubscription
	}
	if err != nil {
		return Event{}, false, err
	}
	now := s.now()
	if !subscriptionLive(sub, req.SubscriptionVersion, now) {
		return Event{}, false, ErrStaleSubscription
	}
	event, created, err = s.store.Insert(ctx, NewEvent{
		SubscriptionID:      id,
		SubscriptionVersion: req.SubscriptionVersion,
		EventID:             req.EventID,
		PayloadRef:          req.PayloadRef,
		EmptyResult:         req.EmptyResult,
		ExpiresAt:           sub.ExpiresAt.Add(s.cfg.ExpiryGrace),
	})
	if pjskdb.IsConstraintError(err) {
		return Event{}, false, ErrStaleSubscription
	}
	if err != nil || !created {
		return event, false, err
	}
	keep := 1
	if s.cfg.ReplayPolicy == config.EventsReplayAll {
		keep = s.cfg.ReplayMax
	}
	if superseded, policyErr := s.store.ApplyReplayPolicy(ctx, event, keep, now); policyErr != nil {
		s.log.WarnContext(ctx, "realtime replay policy failed",
			"event", "realtime_replay_policy_failed",
			"subscription_id", id,
			"error_type", fmt.Sprintf("%T", policyErr))
	} else if superseded > 0 {
		s.log.InfoContext(ctx, "realtime events superseded",
			"event", "realtime_superseded", "subscription_id", id, "count", superseded)
	}
	s.deliverLive(ctx, event, false)
	return event, true, nil
}

// deliverLive claims one delivery and queues the event on the live streams
// of its key. Without a live stream the event waits for the replay.
func (s *Service) deliverLive(ctx context.Context, event Event, redelivery bool) int {
	if !s.hub.Online(event.Key()) {
		return 0
	}
	ok, err := s.store.Claim(ctx, event.ID, s.cfg.MaxDeliveries, s.now())
	if err != nil {
		s.log.WarnContext(ctx, "realtime delivery claim failed",
			"event", "realtime_claim_failed",
			"subscription_id", event.SubscriptionID,
			"error_type", fmt.Sprintf("%T", err))
		return 0
	}
	if !ok {
		return 0
	}
	return s.hub.Deliver(event, redelivery)
}

// Close ends every stream of the subscription version and returns how many
// it closed.
func (s *Service) Close(subscriptionID int, version string) int {
	return s.hub.Close(Key{SubscriptionID: subscriptionID, Version: strings.TrimSpace(version)}, CloseReasonRequested)
}

// Run starts the sweeps and blocks until ctx is done, then ends every
// stream with a retry hint. Call it once per Service.
func (s *Service) Run(ctx context.Context) {
	sweep := time.NewTicker(s.cfg.SweepInterval)
	defer sweep.Stop()
	gc := time.NewTicker(s.cfg.GCInterval)
	defer gc.Stop()
	s.collectGarbage(ctx)
	for {
		select {
		case <-ctx.Done():
			s.Shutdown()
			return
		case <-sweep.C:
			s.Sweep(ctx)
		case <-gc.C:
			s.collectGarbage(ctx)
		}
	}
}

// Shutdown refuses new streams and ends the live ones with a retry hint.
func (s *Service) Shutdown() {
	s.stopping.Store(true)
	if closed := s.hub.CloseAll(CloseReasonShutdown); closed > 0 {
		s.log.Info("realtime streams closed for shutdown", "event", "realtime_shutdown", "streams", closed)
	}
	if s.forwarder != nil {
		s.forwarder.wait(dbTimeout)
	}
}

// Sweep closes streams whose subscription version is no longer active and
// redelivers overdue events on the remaining live streams.
func (s *Service) Sweep(ctx context.Context) {
	keys := s.hub.Keys()
	if len(keys) == 0 || s.readOnly() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()
	now := s.now()
	ids := make([]int, 0, len(keys))
	seen := make(map[int]struct{}, len(keys))
	for _, key := range keys {
		if _, ok := seen[key.SubscriptionID]; !ok {
			seen[key.SubscriptionID] = struct{}{}
			ids = append(ids, key.SubscriptionID)
		}
	}
	subs, err := s.store.Subscriptions(ctx, ids)
	if err != nil {
		s.log.WarnContext(ctx, "realtime stale check failed", "event", "realtime_sweep_failed", "error_type", fmt.Sprintf("%T", err))
		return
	}
	live := make(map[Key]bool, len(keys))
	for _, key := range keys {
		if subscriptionLive(subs[key.SubscriptionID], key.Version, now) {
			live[key] = true
			continue
		}
		if closed := s.hub.Close(key, CloseReasonStale); closed > 0 {
			s.log.InfoContext(ctx, "realtime stale streams closed",
				"event", "realtime_stale_closed", "subscription_id", key.SubscriptionID, "streams", closed)
		}
	}

	cutoff := now.Add(-s.cfg.RedeliverAfter)
	if _, err := s.store.SupersedeExhausted(ctx, s.cfg.MaxDeliveries, cutoff, now); err != nil {
		s.log.WarnContext(ctx, "realtime delivery limit sweep failed", "event", "realtime_sweep_failed", "error_type", fmt.Sprintf("%T", err))
	}
	overdue, err := s.store.Overdue(ctx, ids, cutoff)
	if err != nil {
		s.log.WarnContext(ctx, "realtime redelivery query failed", "event", "realtime_sweep_failed", "error_type", fmt.Sprintf("%T", err))
		return
	}
	for _, event := range overdue {
		if !live[event.Key()] {
			continue
		}
		if delivered := s.deliverLive(ctx, event, true); delivered > 0 {
			s.log.InfoContext(ctx, "realtime event redelivered",
				"event", "realtime_redelivered", "subscription_id", event.SubscriptionID, "event_row", event.ID)
		}
	}
}

func (s *Service) collectGarbage(ctx context.Context) {
	if s.readOnly() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()
	deleted, err := s.store.DeleteExpired(ctx, s.now().Add(-s.cfg.Retention))
	if err != nil {
		s.log.WarnContext(ctx, "realtime event GC failed", "event", "realtime_gc_failed", "error_type", fmt.Sprintf("%T", err))
		return
	}
	if deleted > 0 {
		s.log.InfoContext(ctx, "realtime events deleted", "event", "realtime_gc", "rows", deleted)
	}
}
