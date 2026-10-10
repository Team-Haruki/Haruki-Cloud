package realtime

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	json "haruki-cloud/internal/jsonutil"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/sse"
)

// BirthdayMonitorEventName is the SSE event name the Client listens for.
const BirthdayMonitorEventName = "birthday_monitor_update"

// Routes served by the events role (and by the main app in embedded mode).
// The paths and JSON field names are a frozen contract with Toolbox, Cloud
// and the Client.
const (
	RouteHealth  = "/healthz"
	RouteSSE     = "/sse"
	RouteIngest  = "/internal/events"
	RouteClose   = "/internal/subscriptions/:subscriptionId/close"
	heartbeatTag = "heartbeat"
)

// wireEvent is the SSE data payload; field names and order match the
// retired standalone gateway.
type wireEvent struct {
	EventID             string `json:"event_id"`
	SubscriptionID      string `json:"subscription_id"`
	SubscriptionVersion string `json:"subscription_version,omitempty"`
	PayloadRef          string `json:"payload_ref,omitempty"`
	EmptyResult         bool   `json:"empty_result"`
}

// Register mounts the realtime routes. closeAuth guards the close route
// (the API role calls it with the shared internal token); nil leaves it
// unguarded, which only tests should do.
func (s *Service) Register(router fiber.Router, closeAuth fiber.Handler) {
	router.Get(RouteHealth, s.handleHealth)
	router.Get(RouteSSE, s.handleSSE)
	router.Post(RouteIngest, s.handleIngest)
	if closeAuth != nil {
		router.Post(RouteClose, closeAuth, s.handleClose)
	} else {
		router.Post(RouteClose, s.handleClose)
	}
}

func errorJSON(c fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(fiber.Map{"error": message})
}

func (s *Service) handleHealth(c fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
}

func (s *Service) handleSSE(c fiber.Ctx) error {
	// Copy out of the request buffers: the stream outlives the handler.
	rawSubscriptionID := strings.Clone(strings.TrimSpace(c.Query("subscription_id")))
	version := strings.TrimSpace(c.Query("subscription_version"))
	if version == "" {
		version = strings.TrimSpace(c.Query("version"))
	}
	version = strings.Clone(version)
	token := strings.TrimSpace(c.Query("token"))
	if rawSubscriptionID == "" || version == "" || token == "" {
		return errorJSON(c, fiber.StatusBadRequest, "subscription_id, subscription_version and token are required")
	}
	// Every failure other than a credential mismatch is a 503: a 401 makes
	// the Client stop reconnecting for good.
	if s.Stopping() || s.readOnly() {
		return errorJSON(c, fiber.StatusServiceUnavailable, "event stream unavailable")
	}
	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	key, err := s.Authenticate(ctx, rawSubscriptionID, version, token)
	cancel()
	if errors.Is(err, ErrInvalidSubscription) {
		s.log.Warn("sse validation rejected", "event", "sse_rejected", "subscription_id", rawSubscriptionID)
		return errorJSON(c, fiber.StatusUnauthorized, "invalid subscription token")
	}
	if err != nil {
		s.log.Error("sse validation failed", "event", "sse_validation_failed", "subscription_id", rawSubscriptionID, "error_type", fmt.Sprintf("%T", err))
		return errorJSON(c, fiber.StatusServiceUnavailable, "subscription validation unavailable")
	}
	stream, err := s.hub.Register(key)
	if err != nil {
		s.log.Warn("sse connection refused", "event", "sse_refused", "subscription_id", key.SubscriptionID, "streams", s.hub.Count())
		return errorJSON(c, fiber.StatusServiceUnavailable, "too many connections")
	}
	lastEventID := parseLastEventID(c.Get(fiber.HeaderLastEventID))
	s.log.Info("sse connected", "event", "sse_connected", "subscription_id", key.SubscriptionID, "last_event_id", lastEventID)
	handler := sse.New(sse.Config{
		Handler: func(_ fiber.Ctx, st *sse.Stream) error {
			return s.serveStream(st, stream, lastEventID)
		},
		Retry:            s.cfg.ClientRetry,
		DisableHeartbeat: true,
	})
	return handler(c)
}

func parseLastEventID(raw string) int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id < 0 {
		return 0
	}
	return id
}

// serveStream replays the pending events, then forwards live ones until the
// stream breaks, the hub closes it, or shutdown sends a retry hint.
func (s *Service) serveStream(st *sse.Stream, c *conn, lastEventID int64) (err error) {
	sent := 0
	reason := "client_gone"
	defer func() {
		s.hub.Unregister(c)
		if closed := c.closeReason(); closed != "" {
			reason = closed
		}
		s.log.Info("sse disconnected",
			"event", "sse_disconnected",
			"subscription_id", c.key.SubscriptionID,
			"reason", reason,
			"events_sent", sent,
			"duration_ms", time.Since(c.connectedAt).Milliseconds())
	}()
	delivered := make(map[int64]bool)
	send := func(event Event) error {
		if err := writeEvent(st, event); err != nil {
			return err
		}
		delivered[event.ID] = true
		sent++
		return nil
	}

	ctx, cancel := context.WithTimeout(st.Context(), dbTimeout)
	replay, replayErr := s.store.Pending(ctx, c.key, lastEventID)
	for _, event := range replay {
		if s.recentlyDelivered(event) {
			continue
		}
		ok, claimErr := s.store.Claim(ctx, event.ID, s.cfg.MaxDeliveries, s.now())
		if claimErr != nil {
			replayErr = claimErr
			break
		}
		if !ok {
			continue
		}
		if err := send(event); err != nil {
			cancel()
			return err
		}
	}
	cancel()
	if replayErr != nil {
		// Undelivered events are picked up by the redelivery sweep.
		s.log.Warn("sse replay failed", "event", "sse_replay_failed", "subscription_id", c.key.SubscriptionID, "error_type", fmt.Sprintf("%T", replayErr))
	}

	heartbeat := time.NewTicker(s.cfg.HeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.closed:
			if c.closeReason() == CloseReasonShutdown {
				_ = st.Retry(s.cfg.ClientRetry)
			}
			return nil
		case <-st.Done():
			return st.Err()
		case d := <-c.queue:
			if !d.redelivery && delivered[d.event.ID] {
				continue
			}
			if err := send(d.event); err != nil {
				return err
			}
		case <-heartbeat.C:
			if err := st.Comment(heartbeatTag); err != nil {
				return err
			}
		}
	}
}

// recentlyDelivered reports whether the event went out within ReplayGrace:
// a reconnecting Client may still be rendering it, so the replay skips it
// and the redelivery sweep sends it again if it is never acknowledged.
func (s *Service) recentlyDelivered(event Event) bool {
	if s.cfg.ReplayGrace <= 0 || event.LastDeliveredAt.IsZero() {
		return false
	}
	return s.now().Sub(event.LastDeliveredAt) < s.cfg.ReplayGrace
}

func writeEvent(st *sse.Stream, event Event) error {
	data, err := json.Marshal(wireEvent{
		EventID:             event.EventID,
		SubscriptionID:      strconv.Itoa(event.SubscriptionID),
		SubscriptionVersion: event.SubscriptionVersion,
		PayloadRef:          event.PayloadRef,
		EmptyResult:         event.EmptyResult,
	})
	if err != nil {
		return err
	}
	return st.Event(sse.Event{
		ID:   strconv.FormatInt(event.ID, 10),
		Name: BirthdayMonitorEventName,
		Data: data,
	})
}

func (s *Service) handleIngest(c fiber.Ctx) error {
	if !s.IngestAuthorized(c.Get(fiber.HeaderAuthorization)) {
		s.log.Warn("internal event unauthorized", "event", "realtime_ingest_unauthorized")
		return errorJSON(c, fiber.StatusUnauthorized, "unauthorized")
	}
	var req IngestRequest
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return errorJSON(c, fiber.StatusBadRequest, "invalid json")
	}
	req.normalize()
	if req.EventID == "" || req.SubscriptionID == "" || req.SubscriptionVersion == "" {
		return errorJSON(c, fiber.StatusBadRequest, "event_id, subscription_id and subscription_version are required")
	}
	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()
	event, created, err := s.Ingest(ctx, req)
	switch {
	case errors.Is(err, ErrStaleSubscription):
		s.log.Info("internal event for stale subscription", "event", "realtime_ingest_stale", "subscription_id", req.SubscriptionID)
		return errorJSON(c, fiber.StatusConflict, "subscription is not active")
	case err != nil:
		s.log.Error("internal event store failed", "event", "realtime_ingest_failed", "subscription_id", req.SubscriptionID, "error_type", fmt.Sprintf("%T", err))
		return errorJSON(c, fiber.StatusServiceUnavailable, "event store unavailable")
	}
	s.log.Info("internal event received",
		"event", "realtime_ingested",
		"subscription_id", event.SubscriptionID,
		"event_row", event.ID,
		"duplicate", !created,
		"empty_result", event.EmptyResult)
	if created && s.forwarder != nil {
		s.forwarder.forward(c.Body())
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
}

type closeBody struct {
	SubscriptionVersion string `json:"subscription_version"`
}

func (s *Service) handleClose(c fiber.Ctx) error {
	version := strings.TrimSpace(c.Query("subscription_version"))
	if version == "" {
		version = strings.TrimSpace(c.Query("version"))
	}
	if body := c.Body(); version == "" && len(strings.TrimSpace(string(body))) > 0 {
		var parsed closeBody
		if err := json.Unmarshal(body, &parsed); err != nil {
			return errorJSON(c, fiber.StatusBadRequest, "invalid json")
		}
		version = strings.TrimSpace(parsed.SubscriptionVersion)
	}
	id, err := strconv.Atoi(strings.TrimSpace(c.Params("subscriptionId")))
	if err != nil || id <= 0 || version == "" {
		return errorJSON(c, fiber.StatusBadRequest, "subscription_id and subscription_version are required")
	}
	closed := s.Close(id, version)
	s.log.Info("subscription close requested", "event", "realtime_close", "subscription_id", id, "closed_clients", closed)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok", "closed_clients": closed})
}
