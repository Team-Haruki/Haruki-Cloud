package pjsk

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"haruki-cloud/api"
	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/shamaton/msgpack/v3"
)

const (
	defaultResponseElectionWindow = 200 * time.Millisecond
	responseElectionPollInterval  = 25 * time.Millisecond
	responseElectionRedisTimeout  = 500 * time.Millisecond
	responseElectionResultTTL     = 30 * time.Second
	responseElectionStateTTL      = dedupInFlightTTL + responseElectionResultTTL
	// Requests do not carry a platform event ID, so retaining a completed
	// content identity for minutes also suppresses legitimate repeated
	// commands. Keep only enough time to absorb immediate duplicate delivery;
	// the longer replay history below still protects multi-bot retries.
	responseElectionCompletedTTL  = 5 * time.Second
	responseElectionHistoryTTL    = 6 * time.Hour
	responseElectionReplayWindow  = 300 * time.Millisecond
	responseElectionPublishBudget = 3 * time.Second
	responseElectionMaxAttempts   = 3
	responseElectionRetryDelay    = 25 * time.Millisecond
	responseElectionWinnerClaim   = time.Second
	responseElectionCloseTimeout  = 5 * time.Second
)

var (
	responseElectionLogger        = logger.NewLoggerFromGlobal("ResponseElection")
	responseElectionFailureLogger = logger.NewLoggerWithCommandWriter("ResponseElection", "WARN")
	errResponseElectionAborted    = errors.New("response election publication aborted")
	errResponseElectionUnknown    = errors.New("response election publication state unknown")
)

var joinResponseElectionScript = redis.NewScript(`
local time_parts = redis.call("TIME")
local now_ms = (tonumber(time_parts[1]) * 1000) + math.floor(tonumber(time_parts[2]) / 1000)

if redis.call("EXISTS", KEYS[1]) == 0 then
    if redis.call("EXISTS", KEYS[3]) ~= 0 then
        return {0, 0}
    end

    local history_candidates = tonumber(redis.call("GET", KEYS[5]) or "0")
    local expected_candidates = tonumber(ARGV[5] or "0")
    if history_candidates > 1 and expected_candidates > 1 then
        if not redis.call("SET", KEYS[4], ARGV[1], "PX", ARGV[7], "NX") then
            return {0, 0}
        end
        local replay_deadline_ms = now_ms + tonumber(ARGV[6])
		redis.call("HSET", KEYS[1],
		    "owner", ARGV[1],
		    "executor_bot", "",
		    "seed", ARGV[1],
		    "candidate_peak", "1",
		    "deadline_ms", replay_deadline_ms,
            "replay_pending", "1",
            "ready", "0",
            "closed", "0",
            "consumed", "0")
        redis.call("HSET", KEYS[2], ARGV[2], ARGV[1])
        redis.call("PEXPIRE", KEYS[1], ARGV[7])
        redis.call("PEXPIRE", KEYS[2], ARGV[7])
        return {3, tonumber(ARGV[6])}
    end

    if not redis.call("SET", KEYS[4], ARGV[1], "PX", ARGV[4], "NX") then
        return {0, 0}
    end

    -- ARGV[5] is the roster-informed expected candidate count (0 = unknown).
    -- A single expected candidate needs no window: the creator is the only
    -- bot that will ever observe this event, so it can respond immediately.
    local window_ms = tonumber(ARGV[3])
    if tonumber(ARGV[5] or "0") == 1 then
        window_ms = 0
    end
    local deadline_ms = now_ms + window_ms
	redis.call("HSET", KEYS[1],
	    "owner", ARGV[1],
	    "executor_bot", ARGV[2],
	    "seed", ARGV[1],
	    "candidate_peak", "1",
        "deadline_ms", deadline_ms,
        "ready", "0",
        "closed", "0",
        "consumed", "0")
    redis.call("HSET", KEYS[2], ARGV[2], ARGV[1])
    redis.call("PEXPIRE", KEYS[1], ARGV[4])
    redis.call("PEXPIRE", KEYS[2], ARGV[4])
    return {1, window_ms}
end

local deadline_ms = tonumber(redis.call("HGET", KEYS[1], "deadline_ms") or "0")
local existing_token = redis.call("HGET", KEYS[2], ARGV[2])
if existing_token then
    if existing_token ~= ARGV[1] then
        return {0, math.max(0, deadline_ms - now_ms)}
    end
    if redis.call("HGET", KEYS[1], "owner") == ARGV[1] then
        return {1, math.max(0, deadline_ms - now_ms)}
    end
    return {2, math.max(0, deadline_ms - now_ms)}
end

if redis.call("HGET", KEYS[1], "replay_pending") == "1" then
    if now_ms >= deadline_ms then
        return {0, 0}
    end
	redis.call("HSET", KEYS[2], ARGV[2], ARGV[1])
	local candidate_count = redis.call("HLEN", KEYS[2])
	if candidate_count > tonumber(redis.call("HGET", KEYS[1], "candidate_peak") or "0") then
	    redis.call("HSET", KEYS[1], "candidate_peak", candidate_count)
	end
	if candidate_count < 2 then
        return {3, deadline_ms - now_ms}
    end

    local window_ms = tonumber(ARGV[3])
    redis.call("SET", KEYS[4], ARGV[1], "PX", ARGV[4], "XX")
    redis.call("HSET", KEYS[1],
        "owner", ARGV[1],
        "executor_bot", ARGV[2],
        "seed", ARGV[1],
        "deadline_ms", now_ms + window_ms,
        "replay_pending", "0")
    redis.call("PEXPIRE", KEYS[1], ARGV[4])
    redis.call("PEXPIRE", KEYS[2], ARGV[4])
    return {1, window_ms}
end

if redis.call("HGET", KEYS[1], "closed") == "1" or now_ms >= deadline_ms then
    return {0, 0}
end
redis.call("HSET", KEYS[2], ARGV[2], ARGV[1])
local candidate_count = redis.call("HLEN", KEYS[2])
if candidate_count > tonumber(redis.call("HGET", KEYS[1], "candidate_peak") or "0") then
    redis.call("HSET", KEYS[1], "candidate_peak", candidate_count)
end
return {2, deadline_ms - now_ms}
`)

var publishResponseElectionScript = redis.NewScript(`
if redis.call("HGET", KEYS[1], "owner") ~= ARGV[1] then
    return 0
end
if redis.call("HGET", KEYS[1], "ready") == "1" then
    return 1
end
if redis.call("HGET", KEYS[1], "closed") == "1" then
    return 0
end

redis.call("HSET", KEYS[1],
    "ready", "1",
    "force_executor", ARGV[2],
    "result", ARGV[3],
    "result_serialize_nanos", ARGV[5])
redis.call("PEXPIRE", KEYS[1], ARGV[4])
redis.call("PEXPIRE", KEYS[2], ARGV[4])
redis.call("PEXPIRE", KEYS[3], ARGV[4])
return 1
`)

var abortResponseElectionScript = redis.NewScript(`
if redis.call("HGET", KEYS[1], "owner") ~= ARGV[1] then
    return 0
end
if redis.call("HGET", KEYS[1], "ready") == "1" then
    return 2
end
if redis.call("HGET", KEYS[1], "closed") ~= "1" then
    redis.call("HSET", KEYS[1], "closed", "1", "aborted", "1")
    redis.call("HDEL", KEYS[1], "result")
    redis.call("SET", KEYS[3], "1", "PX", ARGV[2])
    if redis.call("GET", KEYS[4]) == ARGV[1] then
        redis.call("DEL", KEYS[4])
    end
    redis.call("PEXPIRE", KEYS[1], ARGV[3])
    redis.call("PEXPIRE", KEYS[2], ARGV[3])
end
return 1
`)

var decideResponseElectionScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 0 then
    return {-1}
end
local function release_legacy_lock()
    local owner = redis.call("HGET", KEYS[1], "owner")
    if owner and redis.call("GET", KEYS[4]) == owner then
        redis.call("DEL", KEYS[4])
    end
end
if redis.call("HGET", KEYS[1], "closed") == "1" then
    if redis.call("HGET", KEYS[1], "consumed_token") == ARGV[1] and
       redis.call("HGET", KEYS[1], "consumed_bot") == ARGV[2] then
        local consumed_result = redis.call("HGET", KEYS[1], "result")
        if consumed_result then
            return {1, consumed_result, redis.call("HGET", KEYS[1], "result_serialize_nanos") or "0"}
        end
    end
    return {0}
end

local time_parts = redis.call("TIME")
local now_ms = (tonumber(time_parts[1]) * 1000) + math.floor(tonumber(time_parts[2]) / 1000)
local deadline_ms = tonumber(redis.call("HGET", KEYS[1], "deadline_ms") or "0")
if now_ms < deadline_ms or redis.call("HGET", KEYS[1], "ready") ~= "1" then
    return {2}
end

local winner_bot = redis.call("HGET", KEYS[1], "winner_bot")
local winner_token = redis.call("HGET", KEYS[1], "winner_token")
if winner_token then
    local claim_deadline_ms = tonumber(redis.call("HGET", KEYS[1], "winner_claim_deadline_ms") or "0")
    if claim_deadline_ms > 0 and now_ms >= claim_deadline_ms then
        redis.call("HDEL", KEYS[2], winner_bot)
        redis.call("HDEL", KEYS[1], "winner_bot", "winner_token", "winner_claim_deadline_ms")
        winner_bot = nil
        winner_token = nil
    end
end
if not winner_token then
    if redis.call("HGET", KEYS[1], "force_executor") == "1" then
        winner_bot = redis.call("HGET", KEYS[1], "executor_bot")
        winner_token = redis.call("HGET", KEYS[2], winner_bot)
    else
        local seed = redis.call("HGET", KEYS[1], "seed") or ""
        local candidates = redis.call("HGETALL", KEYS[2])
        local best_score = nil
        for index = 1, #candidates, 2 do
            local candidate_bot = candidates[index]
            local candidate_token = candidates[index + 1]
            local score = redis.sha1hex(seed .. "|" .. candidate_bot)
            if not best_score or score < best_score or (score == best_score and candidate_bot < winner_bot) then
                best_score = score
                winner_bot = candidate_bot
                winner_token = candidate_token
            end
        end
    end

    if not winner_token then
        redis.call("HSET", KEYS[1], "closed", "1")
        redis.call("SET", KEYS[3], "1", "PX", ARGV[3])
        release_legacy_lock()
        redis.call("PEXPIRE", KEYS[1], ARGV[4])
        redis.call("PEXPIRE", KEYS[2], ARGV[4])
        return {0}
    end
    redis.call("HSET", KEYS[1],
        "winner_bot", winner_bot,
        "winner_token", winner_token,
        "winner_claim_deadline_ms", now_ms + tonumber(ARGV[5]))
end

if winner_token ~= ARGV[1] or winner_bot ~= ARGV[2] then
    return {2}
end

local result = redis.call("HGET", KEYS[1], "result")
if not result then
    return {2}
end
local joined = {}
local candidate_fields = redis.call("HGETALL", KEYS[2])
for index = 1, #candidate_fields, 2 do
    joined[#joined + 1] = candidate_fields[index]
end
return {1, result, redis.call("HGET", KEYS[1], "result_serialize_nanos") or "0", table.concat(joined, ",")}
`)

var completeResponseElectionScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 0 then
    return 0
end
if redis.call("HGET", KEYS[1], "closed") == "1" then
    if redis.call("HGET", KEYS[1], "consumed_token") == ARGV[1] and
       redis.call("HGET", KEYS[1], "consumed_bot") == ARGV[2] then
        return 1
    end
    return 0
end
if redis.call("HGET", KEYS[1], "winner_token") ~= ARGV[1] or
   redis.call("HGET", KEYS[1], "winner_bot") ~= ARGV[2] then
    return 0
end

redis.call("HSET", KEYS[1],
    "consumed", "1",
    "consumed_bot", ARGV[2],
    "consumed_token", ARGV[1],
    "closed", "1")
redis.call("SET", KEYS[3], "1", "PX", ARGV[3])
local owner = redis.call("HGET", KEYS[1], "owner")
if owner and redis.call("GET", KEYS[4]) == owner then
    redis.call("DEL", KEYS[4])
end
local candidate_count = tonumber(redis.call("HGET", KEYS[1], "candidate_peak") or "0")
if candidate_count == 0 then
    candidate_count = redis.call("HLEN", KEYS[2])
end
redis.call("SET", KEYS[5], tostring(candidate_count), "PX", ARGV[5])
redis.call("PEXPIRE", KEYS[1], ARGV[4])
redis.call("PEXPIRE", KEYS[2], ARGV[4])
return 1
`)

var leaveResponseElectionScript = redis.NewScript(`
if redis.call("HGET", KEYS[2], ARGV[1]) ~= ARGV[2] then
    return 0
end
redis.call("HDEL", KEYS[2], ARGV[1])
if redis.call("HGET", KEYS[1], "closed") ~= "1" and
   redis.call("HGET", KEYS[1], "winner_token") == ARGV[2] then
    redis.call("HDEL", KEYS[1], "winner_bot", "winner_token", "winner_claim_deadline_ms")
end
return 1
`)

type responseElectionRole uint8

const (
	responseElectionRejected responseElectionRole = iota
	responseElectionExecutor
	responseElectionFollower
	responseElectionDirect
)

type responseElectionLease struct {
	role          responseElectionRole
	stateKey      string
	candidatesKey string
	rateKey       string
	legacyLockKey string
	historyKey    string
	token         string
	botID         string
	deadline      time.Time
	// expectedCandidates is the roster-informed candidate count passed to the
	// join script (0 = roster unknown); windowSkipped reports whether the
	// single-candidate fast path zeroed the window for this election.
	expectedCandidates int
	windowSkipped      bool
}

type sharedCommandMetadata struct {
	Command       string `msgpack:"command"`
	CommandPath   string `msgpack:"command_path"`
	Module        string `msgpack:"module"`
	Mode          string `msgpack:"mode"`
	Region        string `msgpack:"region"`
	Outcome       string `msgpack:"outcome"`
	ErrorType     string `msgpack:"error_type"`
	ExecutorBotID string `msgpack:"executor_bot_id"`
}

type sharedCommandOperation struct {
	Name       string `msgpack:"name"`
	Count      int    `msgpack:"count"`
	TotalNanos int64  `msgpack:"total_nanos"`
	MaxNanos   int64  `msgpack:"max_nanos"`
}

type sharedCommandResult struct {
	Response      encodedBotResponse       `msgpack:"response"`
	Metadata      sharedCommandMetadata    `msgpack:"metadata"`
	Operations    []sharedCommandOperation `msgpack:"operations,omitempty"`
	ForceExecutor bool                     `msgpack:"force_executor"`
}

type responseElectionDecision struct {
	visible bool
	result  sharedCommandResult
	reason  string
	// expectedCandidates / windowSkipped surface the roster fast path in the
	// election completion log; both stay zero when the roster is disabled.
	expectedCandidates int
	windowSkipped      bool
	// joinedBots is the candidate set snapshot taken atomically inside the
	// decide script when the winner is claimed; only the selected participant
	// receives it, for roster reconciliation.
	joinedBots []string
	handoff    *responseElectionHandoff
}

type responseElectionRequest struct {
	Request      BotCommandRequest
	BotID        string
	DeferHandoff bool
}

type responseElectionHandoff struct {
	coordinator *ResponseElectionCoordinator
	lease       responseElectionLease
	request     responseElectionRequest
	joinedBots  []string
	once        sync.Once
	err         error
}

type commandResponseElection interface {
	Coordinate(
		ctx context.Context,
		request responseElectionRequest,
		execute func(context.Context) sharedCommandResult,
	) responseElectionDecision
	Close()
}

// requestGuardResponseElection preserves the existing first-arrival behavior
// when response election is explicitly disabled. The legacy guard still owns
// event deduplication and the post-response cooldown in that mode.
type requestGuardResponseElection struct {
	guard commandRequestGuard
}

func (e *requestGuardResponseElection) Coordinate(
	ctx context.Context,
	request responseElectionRequest,
	execute func(context.Context) sharedCommandResult,
) responseElectionDecision {
	if execute == nil {
		return responseElectionDecision{reason: "missing_executor"}
	}
	lease := acquireRequestGuard(ctx, e.guard, request.Request)
	if !lease.proceed {
		return responseElectionDecision{reason: "guard_rejected"}
	}
	defer markRequestGuardComplete(ctx, e.guard, request.Request, lease)
	return responseElectionDecision{
		visible: true,
		result:  executeSharedCommandWithTrace(ctx, execute, time.Now),
		reason:  "guard_selected",
	}
}

func (*requestGuardResponseElection) Close() {}

type ResponseElectionCoordinator struct {
	redis        *redis.Client
	window       time.Duration
	roster       *responseElectionRoster
	worker       context.Context
	cancel       context.CancelFunc
	workers      sync.WaitGroup
	workersMu    sync.Mutex
	closing      bool
	closeOnce    sync.Once
	shutdownDone chan struct{}
	now          func() time.Time
	after        func(time.Duration) <-chan time.Time
	pollInterval time.Duration
}

func NewResponseElectionCoordinator(parent context.Context, client *redis.Client, window time.Duration) *ResponseElectionCoordinator {
	if client == nil || window < 0 {
		return nil
	}
	if parent == nil {
		parent = context.Background()
	}
	if window == 0 {
		window = defaultResponseElectionWindow
	}
	if window < time.Millisecond {
		window = time.Millisecond
	}
	worker, cancel := context.WithCancel(parent)
	return &ResponseElectionCoordinator{
		redis:        client,
		window:       window,
		worker:       worker,
		cancel:       cancel,
		shutdownDone: make(chan struct{}),
		now:          time.Now,
		after:        time.After,
		pollInterval: responseElectionPollInterval,
	}
}

// WithRoster enables the learned per-group roster fast path. Nil-safe on both
// sides so the roster remains an optional, config-gated dependency.
func (c *ResponseElectionCoordinator) WithRoster(roster *responseElectionRoster) *ResponseElectionCoordinator {
	if c == nil {
		return nil
	}
	c.roster = roster
	return c
}

func coordinateCommandResponse(
	ctx context.Context,
	coordinator commandResponseElection,
	request responseElectionRequest,
	execute func(context.Context) sharedCommandResult,
) responseElectionDecision {
	if execute == nil {
		return responseElectionDecision{reason: "missing_executor"}
	}
	if coordinator == nil {
		return responseElectionDecision{
			visible: true,
			result:  executeSharedCommandWithTrace(ctx, execute, time.Now),
			reason:  "direct",
		}
	}
	return coordinator.Coordinate(ctx, request, execute)
}

func (c *ResponseElectionCoordinator) Coordinate(
	ctx context.Context,
	request responseElectionRequest,
	execute func(context.Context) sharedCommandResult,
) responseElectionDecision {
	if execute == nil {
		return responseElectionDecision{reason: "missing_executor"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil || c.redis == nil {
		return responseElectionDecision{
			visible: true,
			result:  executeSharedCommandWithTrace(ctx, execute, time.Now),
			reason:  "direct",
		}
	}
	if ctx.Err() != nil || c.worker.Err() != nil {
		return responseElectionDecision{reason: "canceled"}
	}

	lease, err := c.join(ctx, request)
	if err != nil {
		if ctx.Err() != nil || c.worker.Err() != nil {
			return responseElectionDecision{reason: "canceled"}
		}
		if strings.TrimSpace(request.Request.PlatformGroupID) != "" {
			responseElectionFailureLogger.Warn("response election admission failed closed",
				"event", "response_election_join",
				"outcome", "error",
				"error_type", fmt.Sprintf("%T", err),
			)
			return responseElectionDecision{reason: "admission_fail_closed"}
		}
		responseElectionFailureLogger.Warn("response election admission failed open",
			"event", "response_election_join",
			"outcome", "error",
			"error_type", fmt.Sprintf("%T", err),
		)
		return responseElectionDecision{
			visible: true,
			result:  executeSharedCommandWithTrace(ctx, execute, c.now),
			reason:  "admission_fail_open",
		}
	}
	if lease.role == responseElectionRejected {
		return responseElectionDecision{reason: "rejected"}
	}
	if lease.role == responseElectionDirect {
		return responseElectionDecision{
			visible: true,
			result:  executeSharedCommandWithTrace(ctx, execute, c.now),
			reason:  "direct",
		}
	}
	leaveLease := true
	defer func() {
		if leaveLease {
			c.leave(lease)
		}
	}()

	var executorResult <-chan responseElectionExecutorResult
	if lease.role == responseElectionExecutor {
		executorResult = c.startExecutor(ctx, lease, execute)
	}
	decision := c.await(ctx, lease, executorResult, request.DeferHandoff)
	decision.expectedCandidates = lease.expectedCandidates
	decision.windowSkipped = lease.windowSkipped
	if decision.reason == "selected" && request.DeferHandoff {
		leaveLease = false
		decision.handoff = &responseElectionHandoff{
			coordinator: c,
			lease:       lease,
			request:     request,
			joinedBots:  slices.Clone(decision.joinedBots),
		}
	}
	if decision.reason == "selected" && !request.DeferHandoff && c.roster != nil && !lease.windowSkipped && len(decision.joinedBots) > 0 {
		// Only full-window elections carry demotion evidence: an absent roster
		// member had the whole window to join and did not. The joined set was
		// snapshotted atomically at winner claim, so follower cleanup cannot
		// race it. Zero-window elections skip reconciliation entirely, and so
		// do consumed-replay decides (retry after a committed-but-lost reply):
		// those return no snapshot, and a genuine fresh consume always
		// includes at least the winner itself, so an empty joined set proves
		// a replay whose reconciliation evidence is unreliable.
		c.roster.reconcile(
			c.worker,
			request.Request.Platform,
			request.Request.PlatformGroupID,
			decision.joinedBots,
			lease.botID,
		)
	}
	return decision
}

type responseElectionExecutorResult struct {
	result sharedCommandResult
	err    error
}

// Execution ownership is intentionally not transferred before a result is
// published: some commands mutate bindings or settings, so replaying after an
// executor crash would trade a bounded lease wait for duplicate side effects.
// Only response consumption is re-elected because it is read-only.
func (c *ResponseElectionCoordinator) startExecutor(
	parent context.Context,
	lease responseElectionLease,
	execute func(context.Context) sharedCommandResult,
) <-chan responseElectionExecutorResult {
	completed := make(chan responseElectionExecutorResult, 1)
	c.workersMu.Lock()
	if c.closing {
		c.workersMu.Unlock()
		completed <- responseElectionExecutorResult{err: context.Canceled}
		close(completed)
		return completed
	}
	c.workers.Add(1)
	c.workersMu.Unlock()
	go func() {
		defer c.workers.Done()
		detached := logger.DetachedContext(parent)
		executionCtx, cancel := context.WithTimeout(detached, dedupInFlightTTL)
		stopWorkerCancel := context.AfterFunc(c.worker, cancel)
		defer func() {
			stopWorkerCancel()
			cancel()
		}()
		result := executeSharedCommandWithTrace(executionCtx, execute, c.now)
		publishCtx, cancelPublish := context.WithTimeout(detached, responseElectionPublishBudget)
		stopPublishOnShutdown := context.AfterFunc(c.worker, cancelPublish)
		err := c.publish(publishCtx, lease, result)
		stopPublishOnShutdown()
		cancelPublish()
		completed <- responseElectionExecutorResult{result: result, err: err}
		close(completed)
	}()
	return completed
}

// executeSharedCommandWithTrace keeps command phases out of the request's
// top-level phase trace. This is required not only for the normal shared-work
// path, but also for direct, disabled, and Redis fail-open execution where the
// command runs synchronously inside the response_election phase.
func executeSharedCommandWithTrace(
	ctx context.Context,
	execute func(context.Context) sharedCommandResult,
	now func() time.Time,
) sharedCommandResult {
	if now == nil {
		now = time.Now
	}
	startedAt := now()
	executionCtx, trace := commandtrace.WithNewTrace(ctx)
	result := executeSharedCommandSafely(executionCtx, execute)
	result.Operations = append(result.Operations, sharedCommandTraceOperations(trace.Snapshot())...)
	total := now().Sub(startedAt)
	result.Operations = append(result.Operations, sharedCommandOperation{
		Name:       "command.shared_total",
		Count:      1,
		TotalNanos: total.Nanoseconds(),
		MaxNanos:   total.Nanoseconds(),
	})
	return result
}

func executeSharedCommandSafely(ctx context.Context, execute func(context.Context) sharedCommandResult) (result sharedCommandResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			commandtrace.SetErrorType(ctx, "panic")
			attrs := []any{
				"event", "response_election_execute",
				"outcome", "panic",
				"panic_type", fmt.Sprintf("%T", recovered),
			}
			if !harukiConfig.Cfg.Profile.IsProduction() {
				attrs = append(attrs, "stack", string(debug.Stack()))
			}
			logger.ErrorContext(ctx, "shared command execution panicked", attrs...)
			result = sharedCommandPanicResult(ctx)
		}
	}()
	return execute(ctx)
}

func sharedCommandPanicResult(ctx context.Context) sharedCommandResult {
	finishEncode := commandtrace.MeasureOperation(ctx, "response_payload_encode")
	defer finishEncode()
	envelope := newBotResponseEnvelope(fiber.StatusInternalServerError, api.ErrInternalServer)
	encoded, err := encodeBotResponseEnvelope(envelope)
	if err != nil {
		return sharedCommandResult{Metadata: sharedCommandMetadata{Outcome: "error", ErrorType: "panic"}}
	}
	return sharedCommandResult{
		Response: encoded,
		Metadata: sharedCommandMetadata{
			Outcome:   "error",
			ErrorType: "panic",
		},
	}
}

func sharedCommandTraceOperations(snapshot commandtrace.Snapshot) []sharedCommandOperation {
	operations := make([]sharedCommandOperation, 0, len(snapshot.Phases)+len(snapshot.Operations))
	appendStats := func(prefix string, stats []commandtrace.Stats) {
		for _, stat := range stats {
			operations = append(operations, sharedCommandOperation{
				Name:       prefix + stat.Name,
				Count:      stat.Count,
				TotalNanos: stat.Total.Nanoseconds(),
				MaxNanos:   stat.Max.Nanoseconds(),
			})
		}
	}
	appendStats("command.shared.phase.", snapshot.Phases)
	appendStats("command.shared.operation.", snapshot.Operations)
	return operations
}

func mergeSharedCommandOperations(ctx context.Context, operations []sharedCommandOperation) {
	stats := make([]commandtrace.Stats, 0, len(operations))
	for _, operation := range operations {
		stats = append(stats, commandtrace.Stats{
			Name:  operation.Name,
			Count: operation.Count,
			Total: time.Duration(operation.TotalNanos),
			Max:   time.Duration(operation.MaxNanos),
		})
	}
	commandtrace.MergeOperations(ctx, stats)
}

func (c *ResponseElectionCoordinator) join(ctx context.Context, request responseElectionRequest) (responseElectionLease, error) {
	botID := strings.TrimSpace(request.BotID)
	if botID == "" {
		return responseElectionLease{role: responseElectionDirect}, nil
	}
	token, err := newRequestGuardToken()
	if err != nil {
		return responseElectionLease{role: responseElectionDirect}, nil
	}
	identity := responseElectionIdentity(request.Request)
	stateKey := responseElectionStateKey(identity)
	candidatesKey := responseElectionCandidatesKey(identity)
	rateKey := rateLimitKey(request.Request)
	legacyLockKey := dedupKey(request.Request)
	historyKey := responseElectionHistoryKey(identity)
	expected := c.roster.expectedCandidates(c.worker, request.Request.Platform, request.Request.PlatformGroupID, botID)

	operationCtx, cancel := context.WithTimeout(ctx, responseElectionRedisTimeout)
	finishJoin := commandtrace.MeasureOperation(ctx, "response_election.redis_join")
	result, err := joinResponseElectionScript.Run(
		operationCtx,
		c.redis,
		[]string{stateKey, candidatesKey, rateKey, legacyLockKey, historyKey},
		token,
		botID,
		c.window.Milliseconds(),
		c.inFlightTTL().Milliseconds(),
		expected,
		responseElectionReplayWindow.Milliseconds(),
		(responseElectionReplayWindow + responseElectionRedisTimeout).Milliseconds(),
	).Slice()
	finishJoin()
	cancel()
	if err != nil {
		return responseElectionLease{}, err
	}
	// Every observed delivery proves this bot serves the group, including
	// late joins the election rejected — recording those is how a newly added
	// bot becomes part of the expected candidate set.
	c.roster.recordJoin(c.worker, request.Request.Platform, request.Request.PlatformGroupID, botID)
	if len(result) < 2 {
		return responseElectionLease{}, fmt.Errorf("unexpected response election join result")
	}
	status, err := responseElectionInt64(result[0])
	if err != nil {
		return responseElectionLease{}, err
	}
	remainingMS, err := responseElectionInt64(result[1])
	if err != nil {
		return responseElectionLease{}, err
	}
	lease := responseElectionLease{
		stateKey:           stateKey,
		candidatesKey:      candidatesKey,
		rateKey:            rateKey,
		legacyLockKey:      legacyLockKey,
		historyKey:         historyKey,
		token:              token,
		botID:              botID,
		deadline:           c.now().Add(time.Duration(remainingMS) * time.Millisecond),
		expectedCandidates: expected,
		// windowSkipped must reflect the election's ACTUAL window, not this
		// joiner's local expectation: only the creator applies ARGV[5], so a
		// stale-roster follower joining a full-window election must not carry
		// the flag (it gates reconciliation and the rollout metric).
		windowSkipped: status == 1 && expected == 1 && remainingMS == 0,
	}
	switch status {
	case 1:
		lease.role = responseElectionExecutor
	case 2, 3:
		lease.role = responseElectionFollower
	default:
		lease.role = responseElectionRejected
	}
	return lease, nil
}

func (c *ResponseElectionCoordinator) publish(ctx context.Context, lease responseElectionLease, result sharedCommandResult) error {
	serializeStartedAt := time.Now()
	payload, err := msgpack.Marshal(result)
	serializeDuration := time.Since(serializeStartedAt)
	if err != nil {
		return c.reconcileFailedPublish(lease, err)
	}
	forceExecutor := "0"
	if result.ForceExecutor {
		forceExecutor = "1"
	}
	var lastErr error
	publishStartedAt := time.Now()
	attempts := 0

publishAttempts:
	for attempt := 1; attempt <= responseElectionMaxAttempts; attempt++ {
		attempts = attempt
		operationCtx, cancel := context.WithTimeout(ctx, responseElectionRedisTimeout)
		published, publishErr := publishResponseElectionScript.Run(
			operationCtx,
			c.redis,
			[]string{lease.stateKey, lease.candidatesKey, lease.legacyLockKey},
			lease.token,
			forceExecutor,
			payload,
			c.readyTTL().Milliseconds(),
			serializeDuration.Nanoseconds(),
		).Int64()
		cancel()
		if publishErr == nil && published == 1 {
			responseElectionLogger.InfoContext(ctx, "response election result published",
				"event", "response_election_publish",
				"outcome", "ok",
				"attempts", attempts,
				"result_serialize_ms", commandtrace.Milliseconds(serializeDuration),
				"redis_publish_ms", commandtrace.Milliseconds(time.Since(publishStartedAt)),
			)
			return nil
		}
		if publishErr == nil {
			publishErr = errors.New("response election owner changed before publish")
		}
		lastErr = publishErr
		if attempt == responseElectionMaxAttempts || ctx.Err() != nil || c.worker.Err() != nil {
			break
		}
		retry := time.NewTimer(time.Duration(attempt) * responseElectionRetryDelay)
		select {
		case <-retry.C:
		case <-ctx.Done():
			retry.Stop()
			lastErr = ctx.Err()
			break publishAttempts
		case <-c.worker.Done():
			retry.Stop()
			lastErr = c.worker.Err()
			break publishAttempts
		}
	}
	responseElectionFailureLogger.WarnContext(ctx, "response election publish failed",
		"event", "response_election_publish",
		"outcome", "error",
		"attempts", attempts,
		"result_serialize_ms", commandtrace.Milliseconds(serializeDuration),
		"redis_publish_ms", commandtrace.Milliseconds(time.Since(publishStartedAt)),
		"error_type", fmt.Sprintf("%T", lastErr),
	)
	return c.reconcileFailedPublish(lease, lastErr)
}

func (c *ResponseElectionCoordinator) reconcileFailedPublish(lease responseElectionLease, publishErr error) error {
	ctx, cancel := context.WithTimeout(context.Background(), responseElectionRedisTimeout)
	status, err := abortResponseElectionScript.Run(
		ctx,
		c.redis,
		[]string{lease.stateKey, lease.candidatesKey, lease.rateKey, lease.legacyLockKey},
		lease.token,
		rateLimitTTL.Milliseconds(),
		rateLimitTTL.Milliseconds(),
	).Int64()
	cancel()
	if err != nil {
		responseElectionFailureLogger.Warn("response election publish reconciliation failed",
			"event", "response_election_publish_reconcile",
			"outcome", "error",
			"error_type", fmt.Sprintf("%T", err),
		)
		return errors.Join(errResponseElectionUnknown, publishErr)
	}
	if status == 2 {
		return nil
	}
	if status == 1 {
		responseElectionLogger.Info("response election aborted after publish failure",
			"event", "response_election_publish_reconcile",
			"outcome", "aborted",
		)
		return errors.Join(errResponseElectionAborted, publishErr)
	}
	return errors.Join(errResponseElectionUnknown, publishErr)
}

func (c *ResponseElectionCoordinator) await(
	ctx context.Context,
	lease responseElectionLease,
	executorResult <-chan responseElectionExecutorResult,
	deferHandoff bool,
) responseElectionDecision {
	waitCtx, cancel := context.WithTimeout(ctx, c.inFlightTTL())
	defer cancel()

	if wait := lease.deadline.Sub(c.now()); wait > 0 {
		finishWindowWait := commandtrace.MeasureOperation(waitCtx, "response_election.window_wait")
		windowReady := c.after(wait)
		waitingForWindow := true
		for waitingForWindow {
			select {
			case <-windowReady:
				finishWindowWait()
				waitingForWindow = false
			case outcome, ok := <-executorResult:
				if ok && outcome.err != nil {
					finishWindowWait()
					return c.executorPublishFailureDecision(outcome)
				}
				executorResult = nil
			case <-waitCtx.Done():
				finishWindowWait()
				return responseElectionDecision{reason: "canceled"}
			case <-c.worker.Done():
				finishWindowWait()
				return responseElectionDecision{reason: "shutdown"}
			}
		}
	}

	finishResultWait := commandtrace.MeasureOperation(waitCtx, "response_election.result_wait")
	defer finishResultWait()
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()
	for {
		var decision responseElectionDecision
		var waiting bool
		var err error
		if deferHandoff {
			decision, waiting, err = c.claim(waitCtx, lease)
		} else {
			decision, waiting, err = c.decide(waitCtx, lease)
		}
		if err == nil && !waiting {
			return decision
		}
		if err != nil && waitCtx.Err() == nil && c.worker.Err() == nil {
			responseElectionFailureLogger.Warn("response election wait failed",
				"event", "response_election_wait",
				"outcome", "retry",
				"error_type", fmt.Sprintf("%T", err),
			)
		}
		select {
		case outcome, ok := <-executorResult:
			if ok && outcome.err != nil {
				return c.executorPublishFailureDecision(outcome)
			}
			executorResult = nil
		case <-ticker.C:
		case <-waitCtx.Done():
			return responseElectionDecision{reason: "canceled"}
		case <-c.worker.Done():
			return responseElectionDecision{reason: "shutdown"}
		}
	}
}

func (c *ResponseElectionCoordinator) executorPublishFailureDecision(outcome responseElectionExecutorResult) responseElectionDecision {
	if c.worker.Err() != nil || errors.Is(outcome.err, context.Canceled) {
		return responseElectionDecision{result: outcome.result, reason: "shutdown"}
	}
	if errors.Is(outcome.err, errResponseElectionAborted) {
		return responseElectionDecision{visible: true, result: outcome.result, reason: "publish_fail_open"}
	}
	return responseElectionDecision{result: outcome.result, reason: "publish_unknown"}
}

func (c *ResponseElectionCoordinator) decide(ctx context.Context, lease responseElectionLease) (responseElectionDecision, bool, error) {
	decision, waiting, err := c.claim(ctx, lease)
	if err != nil || waiting || !decision.visible {
		return decision, waiting, err
	}
	if err := c.complete(ctx, lease); err != nil {
		return responseElectionDecision{reason: "completion_unknown"}, false, err
	}
	return decision, false, nil
}

func (c *ResponseElectionCoordinator) claim(ctx context.Context, lease responseElectionLease) (responseElectionDecision, bool, error) {
	operationCtx, cancel := context.WithTimeout(ctx, responseElectionRedisTimeout)
	finishDecide := commandtrace.MeasureOperation(ctx, "response_election.redis_decide")
	result, err := decideResponseElectionScript.Run(
		operationCtx,
		c.redis,
		[]string{lease.stateKey, lease.candidatesKey, lease.rateKey, lease.legacyLockKey},
		lease.token,
		lease.botID,
		rateLimitTTL.Milliseconds(),
		rateLimitTTL.Milliseconds(),
		responseElectionWinnerClaim.Milliseconds(),
	).Slice()
	finishDecide()
	cancel()
	if err != nil {
		return responseElectionDecision{}, true, err
	}
	if len(result) == 0 {
		return responseElectionDecision{}, true, fmt.Errorf("empty response election decision")
	}
	status, err := responseElectionInt64(result[0])
	if err != nil {
		return responseElectionDecision{}, true, err
	}
	switch status {
	case 1:
		if len(result) < 2 {
			return responseElectionDecision{}, true, fmt.Errorf("response election winner result missing payload")
		}
		payload, err := responseElectionBytes(result[1])
		if err != nil {
			return responseElectionDecision{}, true, err
		}
		var shared sharedCommandResult
		finishDeserialize := commandtrace.MeasureOperation(ctx, "response_election.result_deserialize")
		err = msgpack.Unmarshal(payload, &shared)
		finishDeserialize()
		if err != nil {
			return responseElectionDecision{}, true, err
		}
		if len(result) >= 3 {
			serializeNanos, err := responseElectionInt64(result[2])
			if err != nil {
				return responseElectionDecision{}, true, err
			}
			shared.Operations = append(shared.Operations, sharedCommandOperation{
				Name:       "response_election.result_serialize",
				Count:      1,
				TotalNanos: serializeNanos,
				MaxNanos:   serializeNanos,
			})
		}
		var joinedBots []string
		if len(result) >= 4 {
			if joined, err := responseElectionBytes(result[3]); err == nil && len(joined) > 0 {
				joinedBots = strings.Split(string(joined), ",")
			}
		}
		return responseElectionDecision{visible: true, result: shared, reason: "selected", joinedBots: joinedBots}, false, nil
	case 0, -1:
		return responseElectionDecision{reason: "not_selected"}, false, nil
	default:
		return responseElectionDecision{}, true, nil
	}
}

func (c *ResponseElectionCoordinator) complete(ctx context.Context, lease responseElectionLease) error {
	operationCtx, cancel := context.WithTimeout(ctx, responseElectionRedisTimeout)
	defer cancel()
	completed, err := completeResponseElectionScript.Run(
		operationCtx,
		c.redis,
		[]string{lease.stateKey, lease.candidatesKey, lease.rateKey, lease.legacyLockKey, lease.historyKey},
		lease.token,
		lease.botID,
		rateLimitTTL.Milliseconds(),
		responseElectionCompletedTTL.Milliseconds(),
		responseElectionHistoryTTL.Milliseconds(),
	).Int64()
	if err != nil {
		return err
	}
	if completed != 1 {
		return errors.New("response election handoff owner changed before completion")
	}
	return nil
}

func (h *responseElectionHandoff) Complete(ctx context.Context) error {
	if h == nil || h.coordinator == nil {
		return nil
	}
	h.once.Do(func() {
		h.err = h.coordinator.complete(ctx, h.lease)
		if h.err == nil && h.coordinator.roster != nil && !h.lease.windowSkipped && len(h.joinedBots) > 0 {
			h.coordinator.roster.reconcile(
				h.coordinator.worker,
				h.request.Request.Platform,
				h.request.Request.PlatformGroupID,
				h.joinedBots,
				h.lease.botID,
			)
		}
		h.coordinator.leave(h.lease)
	})
	return h.err
}

func (h *responseElectionHandoff) Abort() {
	if h == nil || h.coordinator == nil {
		return
	}
	h.once.Do(func() {
		h.coordinator.leave(h.lease)
	})
}

func (c *ResponseElectionCoordinator) inFlightTTL() time.Duration {
	return responseElectionTTL(
		responseElectionStateTTL,
		c.window,
		responseElectionWinnerClaim+responseElectionPublishBudget,
	)
}

func (c *ResponseElectionCoordinator) readyTTL() time.Duration {
	return responseElectionTTL(
		responseElectionResultTTL,
		c.window,
		responseElectionWinnerClaim+responseElectionRedisTimeout,
	)
}

func responseElectionTTL(base, window, margin time.Duration) time.Duration {
	const maxDuration = time.Duration(1<<63 - 1)
	if window > maxDuration-margin {
		return maxDuration
	}
	return max(base, window+margin)
}

func (c *ResponseElectionCoordinator) leave(lease responseElectionLease) {
	if c == nil || c.redis == nil || lease.token == "" || lease.botID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), responseElectionRedisTimeout)
	_, err := leaveResponseElectionScript.Run(
		ctx,
		c.redis,
		[]string{lease.stateKey, lease.candidatesKey},
		lease.botID,
		lease.token,
	).Int64()
	cancel()
	if err != nil {
		responseElectionFailureLogger.Warn("response election candidate cleanup failed",
			"event", "response_election_leave",
			"outcome", "error",
			"error_type", fmt.Sprintf("%T", err),
		)
	}
}

func responseElectionInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected Redis integer type %T", value)
	}
}

func responseElectionBytes(value any) ([]byte, error) {
	switch typed := value.(type) {
	case string:
		return []byte(typed), nil
	case []byte:
		return typed, nil
	default:
		return nil, fmt.Errorf("unexpected Redis bytes type %T", value)
	}
}

func (c *ResponseElectionCoordinator) Close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		c.workersMu.Lock()
		c.closing = true
		c.cancel()
		c.workersMu.Unlock()
		go func() {
			c.workers.Wait()
			close(c.shutdownDone)
		}()
	})
	select {
	case <-c.shutdownDone:
	case <-time.After(responseElectionCloseTimeout):
		responseElectionFailureLogger.Warn("response election shutdown timed out",
			"event", "response_election_shutdown",
			"outcome", "timeout",
		)
	}
}

func logResponseElectionDecision(ctx context.Context, request responseElectionRequest, decision responseElectionDecision) {
	attrs := []any{
		"event", "response_election",
		"outcome", decision.reason,
		"bot_id", request.BotID,
		"selected", decision.visible,
	}
	if decision.result.Metadata.ExecutorBotID != "" {
		attrs = append(attrs, slog.String("executor_bot_id", decision.result.Metadata.ExecutorBotID))
	}
	if decision.expectedCandidates > 0 {
		attrs = append(attrs,
			slog.Int("expected_candidates", decision.expectedCandidates),
			slog.Bool("window_skipped", decision.windowSkipped),
		)
	}
	responseElectionLogger.InfoContext(ctx, "response election completed", attrs...)
}
