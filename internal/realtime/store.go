package realtime

import (
	"context"
	"errors"
	"strings"
	"time"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/mysekaibirthdaysubscription"
	"haruki-cloud/database/pjsk/realtimeevent"

	entsql "entgo.io/ent/dialect/sql"
)

// TopicBirthdayMonitor is the topic of MySekai birthday monitor events.
const TopicBirthdayMonitor = "mysekai_birthday"

// Superseded reasons stored in realtime_events.superseded_reason.
const (
	ReasonReplaced              = "replaced"
	ReasonOverflow              = "overflow"
	ReasonDeliveryLimit         = "delivery_limit"
	ReasonSubscriptionReplaced  = "subscription_replaced"
	ReasonSubscriptionCancelled = "subscription_cancelled"
)

// ErrStoreUnavailable is returned by a Store without a database client.
var ErrStoreUnavailable = errors.New("realtime: event store is not configured")

// Event is one stored realtime event.
type Event struct {
	ID                  int64
	SubscriptionID      int
	SubscriptionVersion string
	EventID             string
	PayloadRef          string
	EmptyResult         bool
	DeliveryCount       int
	// LastDeliveredAt is zero for an event never delivered.
	LastDeliveredAt time.Time
}

// Key identifies the streams of one subscription version.
type Key struct {
	SubscriptionID int
	Version        string
}

// Key returns the stream key the event is delivered on.
func (e Event) Key() Key {
	return Key{SubscriptionID: e.SubscriptionID, Version: e.SubscriptionVersion}
}

func eventFromRow(row *pjskdb.RealtimeEvent) Event {
	var lastDelivered time.Time
	if row.LastDeliveredAt != nil {
		lastDelivered = *row.LastDeliveredAt
	}
	return Event{
		LastDeliveredAt:     lastDelivered,
		ID:                  row.ID,
		SubscriptionID:      row.SubscriptionID,
		SubscriptionVersion: row.SubscriptionVersion,
		EventID:             row.EventID,
		PayloadRef:          row.PayloadRef,
		EmptyResult:         row.EmptyResult,
		DeliveryCount:       row.DeliveryCount,
	}
}

// Store persists realtime events in the PJSK database (realtime_events).
// Both roles use it: the events role writes and delivers, the API role
// acknowledges and supersedes. It holds no state besides the client.
type Store struct {
	db *pjskdb.Client
}

// NewStore returns a Store over db. A nil db yields a Store whose methods
// return ErrStoreUnavailable (writes) or nothing (reads).
func NewStore(db *pjskdb.Client) *Store {
	return &Store{db: db}
}

func (s *Store) ready() bool {
	return s != nil && s.db != nil
}

// Subscription loads the subscription row by id.
func (s *Store) Subscription(ctx context.Context, id int) (*pjskdb.MysekaiBirthdaySubscription, error) {
	if !s.ready() {
		return nil, ErrStoreUnavailable
	}
	return s.db.MysekaiBirthdaySubscription.Get(ctx, id)
}

// Subscriptions loads the subscription rows with the given ids, keyed by id.
func (s *Store) Subscriptions(ctx context.Context, ids []int) (map[int]*pjskdb.MysekaiBirthdaySubscription, error) {
	if !s.ready() {
		return nil, ErrStoreUnavailable
	}
	result := make(map[int]*pjskdb.MysekaiBirthdaySubscription, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := s.db.MysekaiBirthdaySubscription.Query().
		Where(mysekaibirthdaysubscription.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.ID] = row
	}
	return result, nil
}

// NewEvent is an event to insert.
type NewEvent struct {
	SubscriptionID      int
	SubscriptionVersion string
	EventID             string
	PayloadRef          string
	EmptyResult         bool
	ExpiresAt           time.Time
}

// Insert stores the event once per (subscription, version, event_id). It
// returns the stored row and whether this call created it; a repeated
// notification returns the existing row with created=false.
func (s *Store) Insert(ctx context.Context, event NewEvent) (Event, bool, error) {
	if !s.ready() {
		return Event{}, false, ErrStoreUnavailable
	}
	if existing, err := s.findByEventID(ctx, event); err == nil {
		return existing, false, nil
	} else if !pjskdb.IsNotFound(err) {
		return Event{}, false, err
	}
	row, err := s.db.RealtimeEvent.Create().
		SetTopic(TopicBirthdayMonitor).
		SetSubscriptionID(event.SubscriptionID).
		SetSubscriptionVersion(event.SubscriptionVersion).
		SetEventID(event.EventID).
		SetPayloadRef(event.PayloadRef).
		SetEmptyResult(event.EmptyResult).
		SetExpiresAt(event.ExpiresAt).
		Save(ctx)
	if err == nil {
		return eventFromRow(row), true, nil
	}
	if !pjskdb.IsConstraintError(err) {
		return Event{}, false, err
	}
	// A concurrent duplicate won the unique key, or the subscription row
	// vanished (foreign key). Only the former has a row to return.
	existing, findErr := s.findByEventID(ctx, event)
	if findErr != nil {
		return Event{}, false, err
	}
	return existing, false, nil
}

func (s *Store) findByEventID(ctx context.Context, event NewEvent) (Event, error) {
	row, err := s.db.RealtimeEvent.Query().
		Where(
			realtimeevent.SubscriptionID(event.SubscriptionID),
			realtimeevent.SubscriptionVersion(event.SubscriptionVersion),
			realtimeevent.EventID(event.EventID),
		).
		Only(ctx)
	if err != nil {
		return Event{}, err
	}
	return eventFromRow(row), nil
}

// ApplyReplayPolicy supersedes the pending events of newest's subscription
// version that the policy no longer keeps: every older one under "latest",
// everything beyond the newest keep under "all".
func (s *Store) ApplyReplayPolicy(ctx context.Context, newest Event, keep int, now time.Time) (int, error) {
	if !s.ready() {
		return 0, ErrStoreUnavailable
	}
	if keep <= 1 {
		return s.db.RealtimeEvent.Update().
			Where(
				realtimeevent.SubscriptionID(newest.SubscriptionID),
				realtimeevent.SubscriptionVersion(newest.SubscriptionVersion),
				realtimeevent.IDLT(newest.ID),
				realtimeevent.AckedAtIsNil(),
				realtimeevent.SupersededAtIsNil(),
			).
			SetSupersededAt(now).
			SetSupersededReason(ReasonReplaced).
			Save(ctx)
	}
	ids, err := s.db.RealtimeEvent.Query().
		Where(
			realtimeevent.SubscriptionID(newest.SubscriptionID),
			realtimeevent.SubscriptionVersion(newest.SubscriptionVersion),
			realtimeevent.AckedAtIsNil(),
			realtimeevent.SupersededAtIsNil(),
		).
		Order(realtimeevent.ByID(entsql.OrderDesc())).
		Offset(keep).
		IDs(ctx)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	return s.db.RealtimeEvent.Update().
		Where(realtimeevent.IDIn(ids...), realtimeevent.AckedAtIsNil(), realtimeevent.SupersededAtIsNil()).
		SetSupersededAt(now).
		SetSupersededReason(ReasonOverflow).
		Save(ctx)
}

// Pending returns the unacknowledged, non-superseded events of key with an
// id above afterID, oldest first.
func (s *Store) Pending(ctx context.Context, key Key, afterID int64) ([]Event, error) {
	if !s.ready() {
		return nil, ErrStoreUnavailable
	}
	rows, err := s.db.RealtimeEvent.Query().
		Where(
			realtimeevent.SubscriptionID(key.SubscriptionID),
			realtimeevent.SubscriptionVersion(key.Version),
			realtimeevent.IDGT(afterID),
			realtimeevent.AckedAtIsNil(),
			realtimeevent.SupersededAtIsNil(),
		).
		Order(realtimeevent.ByID(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	events := make([]Event, 0, len(rows))
	for _, row := range rows {
		events = append(events, eventFromRow(row))
	}
	return events, nil
}

// Claim records one delivery of the event and reports whether it may be
// sent: false when it was acknowledged, superseded, or already delivered
// maxDeliveries times.
func (s *Store) Claim(ctx context.Context, id int64, maxDeliveries int, now time.Time) (bool, error) {
	if !s.ready() {
		return false, ErrStoreUnavailable
	}
	affected, err := s.db.RealtimeEvent.Update().
		Where(
			realtimeevent.ID(id),
			realtimeevent.AckedAtIsNil(),
			realtimeevent.SupersededAtIsNil(),
			realtimeevent.DeliveryCountLT(maxDeliveries),
		).
		AddDeliveryCount(1).
		SetLastDeliveredAt(now).
		Save(ctx)
	return affected == 1, err
}

// Overdue returns pending events of the given subscriptions that were last
// delivered before cutoff, or never delivered and created before cutoff.
func (s *Store) Overdue(ctx context.Context, subscriptionIDs []int, cutoff time.Time) ([]Event, error) {
	if !s.ready() {
		return nil, ErrStoreUnavailable
	}
	if len(subscriptionIDs) == 0 {
		return nil, nil
	}
	rows, err := s.db.RealtimeEvent.Query().
		Where(
			realtimeevent.SubscriptionIDIn(subscriptionIDs...),
			realtimeevent.AckedAtIsNil(),
			realtimeevent.SupersededAtIsNil(),
			realtimeevent.Or(
				realtimeevent.LastDeliveredAtLT(cutoff),
				realtimeevent.And(realtimeevent.LastDeliveredAtIsNil(), realtimeevent.CreatedAtLT(cutoff)),
			),
		).
		Order(realtimeevent.ByID(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	events := make([]Event, 0, len(rows))
	for _, row := range rows {
		events = append(events, eventFromRow(row))
	}
	return events, nil
}

// SupersedeExhausted supersedes pending events delivered maxDeliveries times
// whose last delivery is older than cutoff.
func (s *Store) SupersedeExhausted(ctx context.Context, maxDeliveries int, cutoff, now time.Time) (int, error) {
	if !s.ready() {
		return 0, ErrStoreUnavailable
	}
	return s.db.RealtimeEvent.Update().
		Where(
			realtimeevent.AckedAtIsNil(),
			realtimeevent.SupersededAtIsNil(),
			realtimeevent.DeliveryCountGTE(maxDeliveries),
			realtimeevent.LastDeliveredAtLT(cutoff),
		).
		SetSupersededAt(now).
		SetSupersededReason(ReasonDeliveryLimit).
		Save(ctx)
}

// Ack marks the event acknowledged. It returns the number of rows changed;
// zero means the event is unknown here or was already acknowledged.
func (s *Store) Ack(ctx context.Context, subscriptionID int, version string, eventID string, now time.Time) (int, error) {
	if !s.ready() {
		return 0, ErrStoreUnavailable
	}
	return s.db.RealtimeEvent.Update().
		Where(
			realtimeevent.SubscriptionID(subscriptionID),
			realtimeevent.SubscriptionVersion(strings.TrimSpace(version)),
			realtimeevent.EventID(strings.TrimSpace(eventID)),
			realtimeevent.AckedAtIsNil(),
		).
		SetAckedAt(now).
		Save(ctx)
}

// SupersedeSubscription supersedes the pending events of every version of
// the subscription except keepVersion (empty keeps none), when it is
// replaced or cancelled.
func (s *Store) SupersedeSubscription(ctx context.Context, subscriptionID int, keepVersion string, reason string, now time.Time) (int, error) {
	if !s.ready() {
		return 0, ErrStoreUnavailable
	}
	update := s.db.RealtimeEvent.Update().
		Where(
			realtimeevent.SubscriptionID(subscriptionID),
			realtimeevent.AckedAtIsNil(),
			realtimeevent.SupersededAtIsNil(),
		)
	if keepVersion = strings.TrimSpace(keepVersion); keepVersion != "" {
		update = update.Where(realtimeevent.SubscriptionVersionNEQ(keepVersion))
	}
	return update.
		SetSupersededAt(now).
		SetSupersededReason(reason).
		Save(ctx)
}

// DeleteExpired removes rows whose expires_at is before cutoff.
func (s *Store) DeleteExpired(ctx context.Context, cutoff time.Time) (int, error) {
	if !s.ready() {
		return 0, ErrStoreUnavailable
	}
	return s.db.RealtimeEvent.Delete().
		Where(realtimeevent.ExpiresAtLT(cutoff)).
		Exec(ctx)
}

// Ping checks that the realtime_events table is reachable.
func (s *Store) Ping(ctx context.Context) error {
	if !s.ready() {
		return ErrStoreUnavailable
	}
	_, err := s.db.RealtimeEvent.Query().Limit(1).IDs(ctx)
	return err
}
