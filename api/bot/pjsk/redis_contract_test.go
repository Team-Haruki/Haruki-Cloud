package pjsk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"
)

// redisContractFingerprint is the pinned fingerprint of every Redis-facing
// piece of the bot dedup / response-election / replay layer: Lua script
// bodies, key prefixes, hash-identity version and TTLs.
//
// Two Cloud generations share one Redis during the AuthV3 dual-run
// (haruki-cloud 2.11.2 alongside this line). Bots in the same group may land
// on either Cloud, so the election state they read and write must stay
// byte-for-byte compatible until the old Cloud is switched off. If this test
// fails you changed that contract: either make the change dual-compatible
// and ship a matching 2.11.x patch to the old Cloud, or wait for cut-over.
// Only then update the fingerprint below.
const redisContractFingerprint = "3d323cdd4032e3b2ad29c573c1a1627a2f92c3a48cc2f84cc6a8fe64d0b969bc"

func TestRedisDedupContractIsFrozenForDualRun(t *testing.T) {
	parts := map[string]string{
		"script.request_guard.acquire":    acquireRequestGuardScript.Hash(),
		"script.request_guard.complete":   completeRequestGuardScript.Hash(),
		"script.election.join":            joinResponseElectionScript.Hash(),
		"script.election.publish":         publishResponseElectionScript.Hash(),
		"script.election.abort":           abortResponseElectionScript.Hash(),
		"script.election.decide":          decideResponseElectionScript.Hash(),
		"script.election.leave":           leaveResponseElectionScript.Hash(),
		"key.dedup":                       dedupKey(BotCommandRequest{}),
		"key.ratelimit.prefix":            strings.SplitAfter(rateLimitKey(BotCommandRequest{}), ":ratelimit:")[0],
		"key.election.prefix":             responseElectionRedisKeyPrefix,
		"key.election.state":              responseElectionStateKey("ID"),
		"key.election.candidates":         responseElectionCandidatesKey("ID"),
		"key.election.roster":             rosterKey("qq", "group"),
		"key.replay.prefix":               "haruki:bot:nonce:",
		"identity.empty_request":          responseElectionIdentity(BotCommandRequest{}),
		"ttl.dedup_in_flight":             dedupInFlightTTL.String(),
		"ttl.rate_limit":                  rateLimitTTL.String(),
		"ttl.election.result":             responseElectionResultTTL.String(),
		"ttl.election.completed":          responseElectionCompletedTTL.String(),
		"ttl.election.state":              responseElectionStateTTL.String(),
		"ttl.election.event_time_skew":    responseElectionEventTimeSkewMax.String(),
		"ttl.election.default_window":     defaultResponseElectionWindow.String(),
		"ttl.replay.default_window":       defaultReplayWindow.String(),
		"ttl.roster":                      rosterTTL.String(),
		"roster.miss_threshold":           fmt.Sprint(rosterMissThreshold),
		"roster.miss_field_prefix":        rosterMissFieldPrefix,
		"election.winner_claim":           responseElectionWinnerClaim.String(),
		"election.publish_budget":         responseElectionPublishBudget.String(),
		"election.post_close_grace_hint":  (3 * time.Second).String(), // ARGV[7] of join = rateLimitTTL
		"election.completed_keep_is_long": fmt.Sprint(responseElectionCompletedTTL > rateLimitTTL),
	}

	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names)

	var builder strings.Builder
	for _, name := range names {
		fmt.Fprintf(&builder, "%s=%s\n", name, parts[name])
	}
	manifest := builder.String()
	sum := sha256.Sum256([]byte(manifest))
	got := hex.EncodeToString(sum[:])

	if got != redisContractFingerprint {
		t.Fatalf("Redis dedup/election contract changed.\n"+
			"This layer is shared with the old Cloud during the dual-run and must stay compatible.\n"+
			"fingerprint: got %s want %s\n\ncurrent contract:\n%s", got, redisContractFingerprint, manifest)
	}
}
