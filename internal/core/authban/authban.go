// Package authban bans a source address from the AuthV3 login endpoint after
// repeated authentication failures.
//
// Failures are counted per source address (IPv6 per /64) in a fixed window in
// Redis, so every Cloud instance shares the count and it survives restarts.
// Reaching the threshold bans the address for BanDuration; each further ban
// inside EscalationWindow doubles the length up to MaxBanDuration. A banned
// address is refused before the Noise handshake and the credential check.
// Bot command routes stay open unless BlockBotRoutes is set, because one
// address often hosts several bots.
package authban

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"log/slog"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/core/secevent"

	"github.com/redis/go-redis/v9"
)

// Reason is a failure kind that counts toward a ban.
type Reason string

const (
	// ReasonAuthFailed is a rejected credential, unknown bot, malformed
	// payload, broken request binding or expired timestamp.
	ReasonAuthFailed Reason = "auth_failed"
	// ReasonReplay is a reused login nonce.
	ReasonReplay Reason = "replay_detected"
	// ReasonHandshake is an empty body or a Noise NK message no configured
	// key could open.
	ReasonHandshake Reason = "noise_handshake_failed"
	// ReasonBuildRejected is an enforced build-policy rejection; it counts
	// only when Config.CountBuildRejected is set.
	ReasonBuildRejected Reason = "build_rejected"
)

// Defaults for zero Config fields.
const (
	DefaultThreshold        = 10
	DefaultWindow           = 10 * time.Minute
	DefaultBanDuration      = 6 * time.Hour
	DefaultMaxBanDuration   = 24 * time.Hour
	DefaultEscalationWindow = 7 * 24 * time.Hour
	DefaultKnownBotTTL      = 7 * 24 * time.Hour

	// maxBotsPerWindow bounds the bot ids remembered for one address and
	// window (they only feed the alert and the admin view).
	maxBotsPerWindow = 20
	// maxBotIDLength bounds a remembered bot id; bot ids are numeric.
	maxBotIDLength = 20
	keyPrefix      = "haruki:authban:"
	activeKey      = keyPrefix + "active"
	// MaxListedBans bounds one admin listing.
	MaxListedBans = 500
)

// builtinNeverBan are never banned: a ban there would hit a proxy or an
// internal caller (and every client behind it) rather than one source.
var builtinNeverBan = []string{
	"0.0.0.0/8",
	"127.0.0.0/8",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"100.64.0.0/10",
	"169.254.0.0/16",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
}

// Config tunes the ban. Zero durations and thresholds take the defaults.
type Config struct {
	Threshold          int
	Window             time.Duration
	BanDuration        time.Duration
	MaxBanDuration     time.Duration
	EscalationWindow   time.Duration
	KnownBotTTL        time.Duration
	CountBuildRejected bool
	ExemptKnownBots    bool
	BlockBotRoutes     bool
	// NeverBan lists extra addresses or CIDRs added to the built-in ranges.
	NeverBan []string
}

// ConfigFromSettings converts the YAML / env settings. enabled is false when
// the operator switched the ban off.
func ConfigFromSettings(s config.AuthIPBanConfig) (cfg Config, enabled bool) {
	return Config{
		Threshold:          s.Threshold,
		Window:             s.Window,
		BanDuration:        s.BanDuration,
		MaxBanDuration:     s.MaxBanDuration,
		EscalationWindow:   s.EscalationWindow,
		KnownBotTTL:        s.KnownBotTTL,
		CountBuildRejected: boolOr(s.CountBuildRejected, true),
		ExemptKnownBots:    boolOr(s.ExemptKnownBots, true),
		BlockBotRoutes:     s.BlockBotRoutes,
		NeverBan:           s.NeverBanCIDRs,
	}, boolOr(s.Enabled, true)
}

func boolOr(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

func (c Config) withDefaults() Config {
	if c.Threshold <= 0 {
		c.Threshold = DefaultThreshold
	}
	if c.Window <= 0 {
		c.Window = DefaultWindow
	}
	if c.BanDuration <= 0 {
		c.BanDuration = DefaultBanDuration
	}
	if c.MaxBanDuration <= 0 {
		c.MaxBanDuration = DefaultMaxBanDuration
	}
	if c.MaxBanDuration < c.BanDuration {
		c.MaxBanDuration = c.BanDuration
	}
	if c.EscalationWindow <= 0 {
		c.EscalationWindow = DefaultEscalationWindow
	}
	if c.KnownBotTTL <= 0 {
		c.KnownBotTTL = DefaultKnownBotTTL
	}
	return c
}

// metrics is exported on /debug/vars as "auth_ip_ban".
var metrics = expvar.NewMap("auth_ip_ban")

const (
	metricFailures    = "failures_counted"
	metricBans        = "bans"
	metricRejected    = "requests_rejected"
	metricKnownExempt = "known_bot_exempted"
	metricExpired     = "bans_expired"
	metricLifted      = "bans_lifted"
	metricRedisErrors = "redis_errors"
)

// Guard is the ban store and policy. A nil *Guard is disabled: every method
// is a no-op that admits the request.
type Guard struct {
	cfg      Config
	rdb      *redis.Client
	neverBan []netip.Prefix
	reporter secevent.Reporter
	logger   *slog.Logger
	now      func() time.Time
}

// New builds a guard. rdb is required; reporter may be nil.
func New(cfg Config, rdb *redis.Client, reporter secevent.Reporter) (*Guard, error) {
	if rdb == nil {
		return nil, errors.New("authban: redis client is required")
	}
	prefixes, err := parseNeverBan(append(slices.Clone(builtinNeverBan), cfg.NeverBan...))
	if err != nil {
		return nil, err
	}
	return &Guard{
		cfg:      cfg.withDefaults(),
		rdb:      rdb,
		neverBan: prefixes,
		reporter: reporter,
		logger:   slog.Default(),
		now:      time.Now,
	}, nil
}

func parseNeverBan(entries []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(entries))
	for _, raw := range entries {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.Contains(raw, "/") {
			p, err := netip.ParsePrefix(raw)
			if err != nil {
				return nil, fmt.Errorf("authban: invalid never_ban_cidrs entry %q: %w", raw, err)
			}
			out = append(out, unmapPrefix(p))
			continue
		}
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			return nil, fmt.Errorf("authban: invalid never_ban_cidrs entry %q: %w", raw, err)
		}
		addr = addr.Unmap().WithZone("")
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}

func unmapPrefix(p netip.Prefix) netip.Prefix {
	addr := p.Addr()
	if addr.Is4In6() && p.Bits() >= 96 {
		return netip.PrefixFrom(addr.Unmap(), p.Bits()-96).Masked()
	}
	return p.Masked()
}

// Config returns the effective configuration (defaults applied).
func (g *Guard) Config() Config {
	if g == nil {
		return Config{}
	}
	return g.cfg
}

// BlocksBotRoutes reports whether banned addresses are also refused on the
// bot command routes.
func (g *Guard) BlocksBotRoutes() bool {
	return g != nil && g.cfg.BlockBotRoutes
}

// subject maps a client address to the counting key: the address itself for
// IPv4, its /64 for IPv6 (one host typically owns a whole /64). ok is false
// for an unparsable address or one inside a never-ban range.
func (g *Guard) subject(ip string) (string, bool) {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return "", false
	}
	addr = addr.Unmap().WithZone("")
	for _, p := range g.neverBan {
		if p.Contains(addr) {
			return "", false
		}
	}
	return subjectOf(addr), true
}

func subjectOf(addr netip.Addr) string {
	if addr.Is4() {
		return addr.String()
	}
	p, _ := addr.Prefix(64)
	return p.String()
}

// NormalizeSubject turns operator input (an address, or an IPv6 /64) into
// the key used by the store.
func NormalizeSubject(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if addr, err := netip.ParseAddr(raw); err == nil {
		return subjectOf(addr.Unmap().WithZone("")), nil
	}
	if p, err := netip.ParsePrefix(raw); err == nil && p.Addr().Is6() && !p.Addr().Is4In6() && p.Bits() == 64 {
		return p.Masked().String(), nil
	}
	return "", errors.New("not an IP address or IPv6 /64")
}

type keys struct {
	fail, bots, ban, level, known string
}

func keysFor(subject string) keys {
	return keys{
		fail:  keyPrefix + "fail:" + subject,
		bots:  keyPrefix + "bots:" + subject,
		ban:   keyPrefix + "ban:" + subject,
		level: keyPrefix + "level:" + subject,
		known: keyPrefix + "known:" + subject,
	}
}

// validBotID keeps only well-formed numeric bot ids for the per-address
// sets, so junk path values never reach Redis.
func validBotID(botID string) string {
	if botID == "" || len(botID) > maxBotIDLength {
		return ""
	}
	for i := 0; i < len(botID); i++ {
		if botID[i] < '0' || botID[i] > '9' {
			return ""
		}
	}
	return botID
}

// Decision is the outcome of Check.
type Decision struct {
	Banned     bool
	RetryAfter time.Duration
}

// Check reports whether ip is banned for botID. Redis errors fail open: the
// login then goes through every other check as usual.
func (g *Guard) Check(ctx context.Context, ip, botID string) Decision {
	if g == nil {
		return Decision{}
	}
	subject, ok := g.subject(ip)
	if !ok {
		return Decision{}
	}
	k := keysFor(subject)
	ttl, err := g.rdb.PTTL(ctx, k.ban).Result()
	if err != nil {
		g.redisError(ctx, "check", err)
		return Decision{}
	}
	if ttl <= 0 {
		return Decision{}
	}
	if bot := validBotID(botID); bot != "" && g.cfg.ExemptKnownBots {
		known, err := g.rdb.SIsMember(ctx, k.known, bot).Result()
		if err != nil {
			g.redisError(ctx, "check_known", err)
			return Decision{}
		}
		if known {
			metrics.Add(metricKnownExempt, 1)
			return Decision{}
		}
	}
	metrics.Add(metricRejected, 1)
	return Decision{Banned: true, RetryAfter: ttl}
}

// recordFailureScript counts one failure and bans the address when the
// count reaches the threshold, all in one atomic step so concurrent
// failures can neither skip the threshold nor ban twice.
//
// KEYS: fail, bots, ban, level, known, active
// ARGV: window_ms, threshold, bot_id, base_ms, max_ms, escalation_ms,
//
//	now_ms, max_bots, subject
//
// Returns {0, count} while counting, {2, 0} when already banned, or
// {1, count, ban_ms, level, bot_id...} when this failure banned the address.
var recordFailureScript = redis.NewScript(`
local bot = ARGV[3]
if bot ~= '' then redis.call('SREM', KEYS[5], bot) end
if redis.call('PTTL', KEYS[3]) > 0 then return {2, 0} end
local n = redis.call('INCR', KEYS[1])
local window = tonumber(ARGV[1])
if n == 1 or redis.call('PTTL', KEYS[1]) < 0 then redis.call('PEXPIRE', KEYS[1], window) end
if bot ~= '' then
  if redis.call('SCARD', KEYS[2]) < tonumber(ARGV[8]) then redis.call('SADD', KEYS[2], bot) end
  local left = redis.call('PTTL', KEYS[1])
  if left > 0 then redis.call('PEXPIRE', KEYS[2], left) end
end
if n < tonumber(ARGV[2]) then return {0, n} end
local level = redis.call('INCR', KEYS[4])
redis.call('PEXPIRE', KEYS[4], tonumber(ARGV[6]))
local dur = tonumber(ARGV[4])
local max = tonumber(ARGV[5])
for i = 2, level do
  if dur >= max then break end
  dur = dur * 2
end
if dur > max then dur = max end
local bots = redis.call('SMEMBERS', KEYS[2])
table.sort(bots)
local now = tonumber(ARGV[7])
redis.call('DEL', KEYS[3])
redis.call('HSET', KEYS[3], 'since', now, 'until', now + dur, 'level', level, 'count', n, 'bots', table.concat(bots, ','))
redis.call('PEXPIRE', KEYS[3], dur)
redis.call('ZADD', KEYS[6], now + dur, ARGV[9])
redis.call('DEL', KEYS[1], KEYS[2])
local out = {1, n, dur, level}
for _, b in ipairs(bots) do out[#out + 1] = b end
return out
`)

// countsAsFailure applies the configured filter.
func (g *Guard) countsAsFailure(reason Reason) bool {
	switch reason {
	case ReasonAuthFailed, ReasonReplay, ReasonHandshake:
		return true
	case ReasonBuildRejected:
		return g.cfg.CountBuildRejected
	}
	return false
}

// RecordFailure counts one failed login from ip and bans the address when it
// reaches the threshold. A failure also drops botID from the address's known
// bots, so a bot that starts failing loses its exemption.
func (g *Guard) RecordFailure(ctx context.Context, ip, botID string, reason Reason) {
	if g == nil || !g.countsAsFailure(reason) {
		return
	}
	subject, ok := g.subject(ip)
	if !ok {
		return
	}
	k := keysFor(subject)
	now := g.now()
	raw, err := recordFailureScript.Run(ctx, g.rdb,
		[]string{k.fail, k.bots, k.ban, k.level, k.known, activeKey},
		g.cfg.Window.Milliseconds(), g.cfg.Threshold, validBotID(botID),
		g.cfg.BanDuration.Milliseconds(), g.cfg.MaxBanDuration.Milliseconds(),
		g.cfg.EscalationWindow.Milliseconds(), now.UnixMilli(), maxBotsPerWindow, subject,
	).Slice()
	if err != nil {
		g.redisError(ctx, "record_failure", err)
		return
	}
	if len(raw) < 2 {
		return
	}
	state, _ := raw[0].(int64)
	if state != 2 {
		metrics.Add(metricFailures, 1)
	}
	if state != 1 || len(raw) < 4 {
		return
	}
	count, _ := raw[1].(int64)
	banMs, _ := raw[2].(int64)
	level, _ := raw[3].(int64)
	bots := make([]string, 0, len(raw)-4)
	for _, v := range raw[4:] {
		if s, ok := v.(string); ok {
			bots = append(bots, s)
		}
	}
	g.banned(ctx, subject, reason, count, time.Duration(banMs)*time.Millisecond, level, bots, now)
}

func (g *Guard) banned(ctx context.Context, subject string, reason Reason, count int64, dur time.Duration, level int64, bots []string, now time.Time) {
	metrics.Add(metricBans, 1)
	until := now.Add(dur).UTC()
	g.logger.LogAttrs(ctx, slog.LevelWarn, "auth ip ban started",
		slog.String("source_ip", subject),
		slog.Int64("failures", count),
		slog.Duration("window", g.cfg.Window),
		slog.Duration("ban_duration", dur),
		slog.Int64("level", level),
		slog.String("until", until.Format(time.RFC3339)),
		slog.String("last_reason", string(reason)),
		slog.Any("bot_ids", bots),
	)
	detail := fmt.Sprintf("%d login failures in %s; banned for %s (ban %d within %s)",
		count, g.cfg.Window, dur, level, g.cfg.EscalationWindow)
	if len(bots) > 0 {
		detail += "; bots " + strings.Join(bots, ",")
	}
	secevent.Report(ctx, g.reporter, secevent.Event{
		Kind:     secevent.KindIPBanned,
		SourceIP: subject,
		Reason:   detail,
		Enforced: true,
		Alert: &secevent.AlertDetail{
			Count:       count,
			Threshold:   g.cfg.Threshold,
			Window:      g.cfg.Window,
			BanDuration: dur,
			BotIDs:      bots,
		},
	})
}

// RecordSuccess remembers botID as known for ip. It does not reset the
// address's failure count: the count belongs to the address, and letting a
// login of one bot wipe it would let a working credential launder failures
// against every other bot from the same place.
func (g *Guard) RecordSuccess(ctx context.Context, ip, botID string) {
	if g == nil || !g.cfg.ExemptKnownBots {
		return
	}
	bot := validBotID(botID)
	if bot == "" {
		return
	}
	subject, ok := g.subject(ip)
	if !ok {
		return
	}
	k := keysFor(subject)
	pipe := g.rdb.TxPipeline()
	pipe.SAdd(ctx, k.known, bot)
	pipe.PExpire(ctx, k.known, g.cfg.KnownBotTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		g.redisError(ctx, "record_success", err)
	}
}

func (g *Guard) redisError(ctx context.Context, op string, err error) {
	metrics.Add(metricRedisErrors, 1)
	g.logger.LogAttrs(ctx, slog.LevelWarn, "auth ip ban store unavailable",
		slog.String("op", op), slog.String("error_type", fmt.Sprintf("%T", err)))
}

// Ban is one active ban.
type Ban struct {
	IP                string    `json:"ip"`
	Since             time.Time `json:"since"`
	Until             time.Time `json:"until"`
	RetryAfterSeconds int64     `json:"retry_after_seconds"`
	Level             int64     `json:"level"`
	Failures          int64     `json:"failures"`
	BotIDs            []string  `json:"bot_ids"`
}

// Status is everything the store holds for one address.
type Status struct {
	IP string `json:"ip"`
	// NeverBanned is true for an address inside a never-ban range.
	NeverBanned bool `json:"never_banned"`
	Ban         *Ban `json:"ban,omitempty"`
	// Failures and BotIDs describe the current counting window.
	Failures               int64    `json:"failures"`
	WindowRemainingSeconds int64    `json:"window_remaining_seconds"`
	BotIDs                 []string `json:"bot_ids"`
	// Level is the number of bans remembered for escalation.
	Level       int64    `json:"level"`
	KnownBotIDs []string `json:"known_bot_ids"`
}

func (g *Guard) readBan(ctx context.Context, subject string) (*Ban, error) {
	k := keysFor(subject)
	fields, err := g.rdb.HGetAll(ctx, k.ban).Result()
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, nil
	}
	ttl, err := g.rdb.PTTL(ctx, k.ban).Result()
	if err != nil {
		return nil, err
	}
	if ttl <= 0 {
		return nil, nil
	}
	ban := &Ban{
		IP:                subject,
		Since:             time.UnixMilli(parseInt(fields["since"])).UTC(),
		Until:             time.UnixMilli(parseInt(fields["until"])).UTC(),
		RetryAfterSeconds: ceilSeconds(ttl),
		Level:             parseInt(fields["level"]),
		Failures:          parseInt(fields["count"]),
		BotIDs:            splitList(fields["bots"]),
	}
	return ban, nil
}

func parseInt(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func splitList(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}

func ceilSeconds(d time.Duration) int64 {
	secs := int64((d + time.Second - 1) / time.Second)
	if secs < 1 {
		secs = 1
	}
	return secs
}

// List returns the active bans, soonest expiry first, at most MaxListedBans.
func (g *Guard) List(ctx context.Context) ([]Ban, error) {
	if g == nil {
		return []Ban{}, nil
	}
	now := strconv.FormatInt(g.now().UnixMilli(), 10)
	subjects, err := g.rdb.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key: activeKey, Start: "(" + now, Stop: "+inf", ByScore: true, Count: MaxListedBans,
	}).Result()
	if err != nil {
		return nil, err
	}
	out := make([]Ban, 0, len(subjects))
	for _, subject := range subjects {
		ban, err := g.readBan(ctx, subject)
		if err != nil {
			return nil, err
		}
		if ban != nil {
			out = append(out, *ban)
		}
	}
	return out, nil
}

// Inspect returns the stored state of one address (or IPv6 /64).
func (g *Guard) Inspect(ctx context.Context, raw string) (Status, error) {
	subject, err := NormalizeSubject(raw)
	if err != nil {
		return Status{}, err
	}
	st := Status{IP: subject, BotIDs: []string{}, KnownBotIDs: []string{}}
	if g == nil {
		return st, nil
	}
	if _, ok := g.subject(subjectAddr(subject)); !ok {
		st.NeverBanned = true
	}
	if st.Ban, err = g.readBan(ctx, subject); err != nil {
		return Status{}, err
	}
	k := keysFor(subject)
	pipe := g.rdb.Pipeline()
	failures := pipe.Get(ctx, k.fail)
	windowTTL := pipe.PTTL(ctx, k.fail)
	bots := pipe.SMembers(ctx, k.bots)
	level := pipe.Get(ctx, k.level)
	known := pipe.SMembers(ctx, k.known)
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return Status{}, err
	}
	st.Failures, _ = failures.Int64()
	if ttl := windowTTL.Val(); ttl > 0 {
		st.WindowRemainingSeconds = ceilSeconds(ttl)
	}
	st.BotIDs = sortedOrEmpty(bots.Val())
	st.Level, _ = level.Int64()
	st.KnownBotIDs = sortedOrEmpty(known.Val())
	return st, nil
}

// subjectAddr returns an address inside subject, for the never-ban check.
func subjectAddr(subject string) string {
	if p, err := netip.ParsePrefix(subject); err == nil {
		return p.Addr().String()
	}
	return subject
}

func sortedOrEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	slices.Sort(values)
	return values
}

// Unban lifts the ban on one address and clears its failure count, window
// bots and escalation level. Known bots are kept. It reports whether a ban
// was active.
func (g *Guard) Unban(ctx context.Context, raw string) (string, bool, error) {
	subject, err := NormalizeSubject(raw)
	if err != nil {
		return "", false, err
	}
	if g == nil {
		return subject, false, nil
	}
	k := keysFor(subject)
	pipe := g.rdb.TxPipeline()
	removed := pipe.Del(ctx, k.ban)
	pipe.Del(ctx, k.fail, k.bots, k.level)
	pipe.ZRem(ctx, activeKey, subject)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", false, err
	}
	wasBanned := removed.Val() > 0
	if wasBanned {
		metrics.Add(metricLifted, 1)
	}
	g.logger.LogAttrs(ctx, slog.LevelWarn, "auth ip ban lifted",
		slog.String("source_ip", subject), slog.Bool("was_banned", wasBanned))
	return subject, wasBanned, nil
}

// SweepExpired logs every ban whose time is up and drops it from the active
// index. Only the instance whose ZREM succeeds logs, so several instances
// sweeping together log each expiry once.
func (g *Guard) SweepExpired(ctx context.Context) {
	if g == nil {
		return
	}
	now := strconv.FormatInt(g.now().UnixMilli(), 10)
	expired, err := g.rdb.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key: activeKey, Start: "-inf", Stop: now, ByScore: true, Count: MaxListedBans,
	}).Result()
	if err != nil {
		g.redisError(ctx, "sweep", err)
		return
	}
	for _, subject := range expired {
		n, err := g.rdb.ZRem(ctx, activeKey, subject).Result()
		if err != nil {
			g.redisError(ctx, "sweep", err)
			return
		}
		if n == 0 {
			continue
		}
		metrics.Add(metricExpired, 1)
		g.logger.LogAttrs(ctx, slog.LevelInfo, "auth ip ban expired", slog.String("source_ip", subject))
	}
}

// RunExpirySweeper calls SweepExpired every interval until ctx ends.
func (g *Guard) RunExpirySweeper(ctx context.Context, interval time.Duration) {
	if g == nil || interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.SweepExpired(ctx)
		}
	}
}
