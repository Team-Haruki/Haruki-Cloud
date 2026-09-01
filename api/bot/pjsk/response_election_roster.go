package pjsk

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// responseElectionRoster learns which bots serve which (platform, group) and
// lets the election creator size the candidate window accordingly:
//
//   - expected == 1 (only the creator is known): the join script sets a zero
//     window, so the single candidate responds without waiting.
//   - expected == 0 (roster unknown/cold): full window, today's behavior.
//   - expected > 1: full window; those groups still hold real elections.
//
// Staleness is fail-safe in both directions. An over-counted roster (a bot
// left the group or went offline) just falls back to the full window until the
// member is demoted; an under-counted roster (a new bot not yet recorded)
// closes early, the late join is rejected, and the user still receives exactly
// one response from the winner while the join itself re-registers the bot for
// subsequent elections.
//
// Demotion is the fast path out of over-counting: after an election that ran a
// full window, the winner compares the roster against the actual candidates,
// increments a miss counter for every absent member, and clears the counter of
// every member that did join; a member missing rosterMissThreshold consecutive
// full-window elections is dropped without waiting for the 7-day TTL. A join
// observed for a bot the local view does not list (fresh demotion or first
// sighting) re-registers it immediately, bypassing the write dedup.
type responseElectionRoster struct {
	redis *redis.Client
	now   func() time.Time

	mu    sync.Mutex
	cache map[string]*rosterCacheState
	order *list.List
}

type rosterCacheState struct {
	element   *list.Element
	members   map[string]struct{}
	fetchedAt time.Time
	// writes dedups recordJoin Redis writes per bot.
	writes map[string]time.Time
	// refreshing guards against stacking async HGETALLs for the same group.
	refreshing bool
}

const (
	rosterTTL           = 7 * 24 * time.Hour
	rosterMissThreshold = 3
	// rosterCacheSoftTTL is how long a cached member set is served without
	// triggering an async refresh; rosterCacheHardTTL is when it stops being
	// trusted entirely (treated as unknown -> full window).
	rosterCacheSoftTTL     = time.Minute
	rosterCacheHardTTL     = 30 * time.Minute
	rosterCacheMaxEntries  = 8192
	rosterWriteDedupWindow = time.Minute
	rosterMissFieldPrefix  = "miss:"
)

func newResponseElectionRoster(client *redis.Client) *responseElectionRoster {
	if client == nil {
		return nil
	}
	return &responseElectionRoster{
		redis: client,
		now:   time.Now,
		cache: make(map[string]*rosterCacheState),
		order: list.New(),
	}
}

// rosterKey hashes the group identity so raw platform group IDs never appear
// in Redis; the {braces} keep the key on one hash slot, matching the election
// key design.
func rosterKey(platform, group string) string {
	h := sha256.New()
	writeResponseElectionHashField(h, []byte(platform))
	writeResponseElectionHashField(h, []byte(group))
	return responseElectionRedisKeyPrefix + "roster:{" + hex.EncodeToString(h.Sum(nil)) + "}"
}

// expectedCandidates returns how many bots are expected to observe a command
// event in this group, or 0 when unknown. DMs are always 1: a direct message
// is only ever delivered to the receiving bot.
func (r *responseElectionRoster) expectedCandidates(ctx context.Context, platform, group, botID string) int {
	if r == nil {
		return 0
	}
	group = strings.TrimSpace(group)
	if group == "" {
		return 1
	}
	platform = strings.TrimSpace(platform)
	key := rosterKey(platform, group)
	now := r.now()

	r.mu.Lock()
	state := r.cacheStateLocked(key)
	age := now.Sub(state.fetchedAt)
	known := !state.fetchedAt.IsZero() && age <= rosterCacheHardTTL
	needsRefresh := (!known || age > rosterCacheSoftTTL) && !state.refreshing
	if needsRefresh {
		state.refreshing = true
	}
	expected := 0
	if known {
		expected = len(state.members)
		if _, ok := state.members[botID]; !ok {
			expected++
		}
	}
	r.mu.Unlock()

	if needsRefresh {
		go r.refresh(ctx, key)
	}
	return expected
}

// recordJoin registers a bot as a live member of the group roster. It is
// called for every join attempt — including rejected late joins, so a newly
// added bot is counted from its first observed delivery onward.
func (r *responseElectionRoster) recordJoin(ctx context.Context, platform, group, botID string) {
	if r == nil || botID == "" {
		return
	}
	group = strings.TrimSpace(group)
	if group == "" {
		return
	}
	key := rosterKey(strings.TrimSpace(platform), group)
	now := r.now()

	r.mu.Lock()
	state := r.cacheStateLocked(key)
	// Dedup only when the local view already lists the bot as a member: a bot
	// missing from a known roster (fresh demotion, first sighting) must
	// re-register immediately, or it would stay excluded from the expected
	// set — and from full-window elections — for up to the dedup window.
	_, knownMember := state.members[botID]
	if knownMember && !state.fetchedAt.IsZero() {
		if last, ok := state.writes[botID]; ok && now.Sub(last) < rosterWriteDedupWindow {
			r.mu.Unlock()
			return
		}
	}
	state.writes[botID] = now
	// Keep the local view coherent so this node counts the bot immediately.
	if !state.fetchedAt.IsZero() {
		state.members[botID] = struct{}{}
	}
	r.mu.Unlock()

	go func() {
		opCtx, cancel := context.WithTimeout(ctx, responseElectionRedisTimeout)
		defer cancel()
		pipe := r.redis.Pipeline()
		pipe.HSet(opCtx, key, botID, strconv.FormatInt(now.UnixMilli(), 10))
		pipe.HDel(opCtx, key, rosterMissFieldPrefix+botID)
		pipe.PExpire(opCtx, key, rosterTTL)
		if _, err := pipe.Exec(opCtx); err != nil && opCtx.Err() == nil {
			responseElectionFailureLogger.Warn("response election roster join write failed",
				"event", "response_election_roster",
				"outcome", "write_error",
				"error_type", fmt.Sprintf("%T", err),
			)
		}
	}()
}

// reconcile runs after a full-window election completed, comparing the roster
// against the candidate set snapshotted atomically at consumption time (plus
// the reconciling bot itself). Members absent from the election accrue misses
// and are demoted at the threshold; the fetched member set also refreshes the
// local cache.
func (r *responseElectionRoster) reconcile(ctx context.Context, platform, group string, joinedBots []string, selfBot string) {
	if r == nil {
		return
	}
	group = strings.TrimSpace(group)
	if group == "" {
		return
	}
	key := rosterKey(strings.TrimSpace(platform), group)
	joined := normalizeJoinedRosterBots(joinedBots, selfBot)
	go r.reconcileRoster(ctx, key, joined)
}

func normalizeJoinedRosterBots(joinedBots []string, selfBot string) map[string]struct{} {
	joined := make(map[string]struct{}, len(joinedBots)+1)
	for _, bot := range joinedBots {
		if bot = strings.TrimSpace(bot); bot != "" {
			joined[bot] = struct{}{}
		}
	}
	if selfBot != "" {
		joined[selfBot] = struct{}{}
	}
	return joined
}

func (r *responseElectionRoster) reconcileRoster(ctx context.Context, key string, joined map[string]struct{}) {
	opCtx, cancel := context.WithTimeout(ctx, responseElectionRedisTimeout)
	defer cancel()
	rosterFields, err := r.redis.HGetAll(opCtx, key).Result()
	if err != nil && err != redis.Nil {
		return
	}
	members, misses := parseRosterFields(rosterFields)
	demoted, err := r.applyRosterReconciliation(opCtx, key, members, misses, joined)
	if err != nil {
		return
	}
	logDemotedRosterMembers(demoted, len(members))
	r.storeMembers(key, members)
}

func parseRosterFields(fields map[string]string) (map[string]struct{}, map[string]int) {
	members := make(map[string]struct{}, len(fields))
	misses := make(map[string]int, len(fields))
	for field, value := range fields {
		missBot, isMiss := strings.CutPrefix(field, rosterMissFieldPrefix)
		if !isMiss {
			members[field] = struct{}{}
			continue
		}
		if count, err := strconv.Atoi(value); err == nil {
			misses[missBot] = count
		}
	}
	return members, misses
}

func (r *responseElectionRoster) applyRosterReconciliation(ctx context.Context, key string, members map[string]struct{}, misses map[string]int, joined map[string]struct{}) ([]string, error) {
	pipeline := r.redis.Pipeline()
	demoted := make([]string, 0, 2)
	mutations := 0
	for member := range members {
		if _, present := joined[member]; present {
			if misses[member] > 0 {
				pipeline.HDel(ctx, key, rosterMissFieldPrefix+member)
				mutations++
			}
			continue
		}
		mutations++
		if misses[member]+1 >= rosterMissThreshold {
			pipeline.HDel(ctx, key, member, rosterMissFieldPrefix+member)
			demoted = append(demoted, member)
			delete(members, member)
			continue
		}
		pipeline.HIncrBy(ctx, key, rosterMissFieldPrefix+member, 1)
	}
	if mutations == 0 {
		return demoted, nil
	}
	if _, err := pipeline.Exec(ctx); err != nil && ctx.Err() == nil && err != redis.Nil {
		responseElectionFailureLogger.Warn("response election roster reconcile failed",
			"event", "response_election_roster", "outcome", "reconcile_error", "error_type", fmt.Sprintf("%T", err))
		return nil, err
	}
	return demoted, nil
}

func logDemotedRosterMembers(demoted []string, remaining int) {
	for _, member := range demoted {
		responseElectionLogger.Info("response election roster member demoted",
			"event", "response_election_roster", "outcome", "demoted",
			"demoted_bot_id", member, "remaining_members", remaining)
	}
}

// refresh loads the roster member set from Redis into the local cache.
func (r *responseElectionRoster) refresh(ctx context.Context, key string) {
	opCtx, cancel := context.WithTimeout(ctx, responseElectionRedisTimeout)
	defer cancel()
	fields, err := r.redis.HGetAll(opCtx, key).Result()
	if err != nil && err != redis.Nil {
		r.mu.Lock()
		if state, ok := r.cache[key]; ok {
			state.refreshing = false
		}
		r.mu.Unlock()
		return
	}
	members := make(map[string]struct{}, len(fields))
	for field := range fields {
		if strings.HasPrefix(field, rosterMissFieldPrefix) {
			continue
		}
		members[field] = struct{}{}
	}
	r.storeMembers(key, members)
}

func (r *responseElectionRoster) storeMembers(key string, members map[string]struct{}) {
	now := r.now()
	r.mu.Lock()
	state := r.cacheStateLocked(key)
	state.members = members
	state.fetchedAt = now
	state.refreshing = false
	r.mu.Unlock()
}

// cacheStateLocked returns (creating if needed) the cache entry for key and
// bounds the cache population LRU-style. Callers must hold r.mu.
func (r *responseElectionRoster) cacheStateLocked(key string) *rosterCacheState {
	if state, ok := r.cache[key]; ok {
		r.order.MoveToFront(state.element)
		return state
	}
	state := &rosterCacheState{
		members: make(map[string]struct{}),
		writes:  make(map[string]time.Time),
	}
	state.element = r.order.PushFront(key)
	r.cache[key] = state
	for len(r.cache) > rosterCacheMaxEntries {
		back := r.order.Back()
		if back == nil {
			break
		}
		r.order.Remove(back)
		delete(r.cache, back.Value.(string))
	}
	return state
}
