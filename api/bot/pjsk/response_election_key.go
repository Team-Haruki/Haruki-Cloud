package pjsk

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strings"
)

const responseElectionRedisKeyPrefix = "haruki:bot:response-election:"

// responseElectionIdentity returns the transport-independent identity of one
// command request. Bot-specific fields, client configuration and OneBot
// segment representation are intentionally excluded so different adapters
// observing the same user command join the same election.
func responseElectionIdentity(req BotCommandRequest) string {
	h := sha256.New()
	writeResponseElectionHashField(h, []byte("v2"))
	writeResponseElectionHashField(h, []byte(strings.ToLower(strings.TrimSpace(req.Platform))))
	writeResponseElectionHashField(h, []byte(strings.TrimSpace(req.PlatformGroupID)))
	writeResponseElectionHashField(h, []byte(strings.TrimSpace(req.PlatformUserID)))
	writeResponseElectionHashField(h, []byte(normalizeCommandIdentityPart(req.MatchedCommand)))
	writeResponseElectionHashField(h, []byte(normalizedCommandArguments(req)))
	return hex.EncodeToString(h.Sum(nil))
}

func normalizeCommandIdentityPart(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func normalizedCommandArguments(req BotCommandRequest) string {
	text := strings.Join(strings.Fields(extractMessageText(req.Message)), " ")
	command := strings.Join(strings.Fields(req.MatchedCommand), " ")
	if command == "" || len(text) < len(command) || !strings.EqualFold(text[:len(command)], command) {
		return text
	}
	if len(text) == len(command) {
		return ""
	}
	if text[len(command)] != ' ' {
		return text
	}
	return strings.TrimSpace(text[len(command)+1:])
}

func responseElectionStateKey(identity string) string {
	return responseElectionRedisKeyPrefix + "{" + identity + "}:state"
}

func responseElectionCandidatesKey(identity string) string {
	return responseElectionRedisKeyPrefix + "{" + identity + "}:candidates"
}

type responseElectionHashWriter interface {
	Write([]byte) (int, error)
}

func writeResponseElectionHashField(dst responseElectionHashWriter, value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = dst.Write(size[:])
	_, _ = dst.Write(value)
}
