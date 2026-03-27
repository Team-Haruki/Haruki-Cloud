// Package integration provides end-to-end integration tests for Haruki-Cloud APIs.
//
// Run with: go test -v -run TestAuth -run TestManifests -run TestBotCommands -run TestExternalAPIs ./integration/ -count=1
package integration_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	corecrypto "haruki-cloud/internal/core/crypto"
	utilscrypto "haruki-cloud/utils/crypto"

	"github.com/vmihailenco/msgpack/v5"
)

// ─── Test Configuration ─────────────────────────────────────────────

const (
	baseURL        = "http://127.0.0.1:6666"
	botID          = "12345678"
	credentialB64  = "CREDENTIAL_VALUE_REDACTED_PLACEHOLDER_00000000000="
	platform       = "qq"
	platformUserID = "QQ_ID_REDACTED"
	region         = "jp"
	gameUserID     = "GAME_USER_ID_REDACTED"

	credentialSignToken = "CREDENTIAL_SIGN_TOKEN_REDACTED_0000000000000000000000000000000000"

	noisePrivKeyHex  = "NOISE_PRIV_KEY_REDACTED_000000000000000000000000000000000000000000"
	serverPubKeyHex  = "NOISE_PUB_KEY_REDACTED_0000000000000000000000000000000000000000000"
)

var (
	sessionToken string
	clientKP     *corecrypto.KeyPair
	serverPubKey []byte
)

// ─── Helpers ────────────────────────────────────────────────────────

func mustDecodeHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// noiseRoundTrip sends an encrypted Noise IK request and decrypts the response.
func noiseRoundTrip(t *testing.T, url string, payload interface{}) ([]byte, int) {
	t.Helper()
	body, err := msgpack.Marshal(payload)
	if err != nil {
		t.Fatalf("msgpack marshal: %v", err)
	}

	nc, err := corecrypto.NewHandshake(clientKP, serverPubKey, true)
	if err != nil {
		t.Fatalf("noise handshake init: %v", err)
	}
	ciphertext, err := nc.EncryptPacket(body)
	if err != nil {
		t.Fatalf("noise encrypt: %v", err)
	}

	req, _ := http.NewRequest("POST", url, bytes.NewReader(ciphertext))
	req.Header.Set("X-Haruki-Bot-Id", botID)
	req.Header.Set("X-Haruki-Bot-Session-Token", sessionToken)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http post %s: %v", url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		// Try to decrypt Noise-encrypted error response
		if len(respBody) > 0 && respBody[0] != '{' && !strings.HasPrefix(string(respBody), "Not Found") {
			if pt, err := nc.DecryptPacket(respBody); err == nil {
				return pt, resp.StatusCode
			}
		}
		return respBody, resp.StatusCode
	}

	plaintext, err := nc.DecryptPacket(respBody)
	if err != nil {
		t.Fatalf("noise decrypt response: %v (raw len=%d)", err, len(respBody))
	}
	return plaintext, resp.StatusCode
}

type botRequest struct {
	Platform        string        `json:"platform" msgpack:"platform"`
	PlatformUserID  string        `json:"platform_user_id" msgpack:"platform_user_id"`
	PlatformGroupID string        `json:"platform_group_id,omitempty" msgpack:"platform_group_id,omitempty"`
	Server          string        `json:"server,omitempty" msgpack:"server,omitempty"`
	MatchedCommand  string        `json:"matched_command" msgpack:"matched_command"`
	Message         []msgSegment  `json:"message" msgpack:"message"`
}

type msgSegment struct {
	Type string            `json:"type" msgpack:"type"`
	Data map[string]string `json:"data" msgpack:"data"`
}

func makeBotReq(cmd, fullText string) botRequest {
	return botRequest{
		Platform:       platform,
		PlatformUserID: platformUserID,
		Server:         region,
		MatchedCommand: cmd,
		Message: []msgSegment{
			{Type: "text", Data: map[string]string{"text": fullText}},
		},
	}
}

func sendBotCommand(t *testing.T, path, cmd, fullText string) ([]byte, int) {
	t.Helper()
	url := fmt.Sprintf("%s/api/v2/bot/%s/pjsk/%s", baseURL, botID, path)
	req := makeBotReq(cmd, fullText)
	return noiseRoundTrip(t, url, req)
}

type botResp struct {
	Status  int         `msgpack:"status" json:"status"`
	Message string      `msgpack:"message" json:"message"`
	Data    interface{} `msgpack:"data" json:"data"`
	Error   string      `msgpack:"error" json:"error"`
}

func parseBotResp(t *testing.T, data []byte) botResp {
	t.Helper()
	var r botResp
	if err := msgpack.Unmarshal(data, &r); err != nil {
		if err2 := json.Unmarshal(data, &r); err2 != nil {
			t.Logf("  raw response (%d bytes): %s", len(data), truncate(data, 200))
		}
	}
	// Extract error from nested data map if present
	if r.Error == "" {
		if dm, ok := r.Data.(map[string]interface{}); ok {
			if e, ok := dm["error"].(string); ok {
				r.Error = e
			}
		}
	}
	return r
}

func truncate(data []byte, max int) string {
	if len(data) <= max {
		return string(data)
	}
	return string(data[:max]) + "..."
}

func summarizeData(data interface{}) string {
	switch v := data.(type) {
	case []interface{}:
		if len(v) == 0 {
			return "[]"
		}
		types := make([]string, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if t, ok := m["type"].(string); ok {
					types = append(types, t)
				}
			}
		}
		return fmt.Sprintf("[%s]", strings.Join(types, ", "))
	case map[string]interface{}:
		return fmt.Sprintf("map[%d keys]", len(v))
	default:
		s := fmt.Sprintf("%v", v)
		if len(s) > 60 {
			return s[:60] + "..."
		}
		return s
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ─── Phase 1: Authentication ────────────────────────────────────────

func TestAuth(t *testing.T) {
	t.Log("=== Phase 1: Bot Authentication ===")

	// Key derivation: server takes the raw credential string bytes (not base64-decoded),
	// copies first 32 bytes into a 32-byte key.
	credRaw := []byte(credentialB64)
	aesKey := make([]byte, 32)
	copy(aesKey, credRaw)

	// Build JWT credential: {bot_id, credential} signed with credentialSignToken
	jwtCred, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"bot_id":     botID,
		"credential": credentialB64,
	}).SignedString([]byte(credentialSignToken))
	if err != nil {
		t.Fatalf("sign JWT credential: %v", err)
	}

	authPayload := fmt.Sprintf(`{"credential":"%s","timestamp":%d}`, jwtCred, time.Now().Unix())
	encrypted, err := utilscrypto.Encrypt([]byte(authPayload), aesKey)
	if err != nil {
		t.Fatalf("encrypt auth payload: %v", err)
	}

	body := fmt.Sprintf(`{"encrypted_payload":"%s"}`, encrypted)
	resp, err := http.Post(baseURL+"/bot/"+botID+"/auth", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("auth request: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("auth failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var authResp struct {
		Message string `json:"message"`
		Data    struct {
			SessionToken string `json:"session_token"`
			ExpiresAt    int64  `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &authResp); err != nil {
		t.Fatalf("parse auth response: %v", err)
	}
	if authResp.Data.SessionToken == "" {
		t.Fatalf("empty session token in response: %s", string(respBody))
	}
	sessionToken = authResp.Data.SessionToken
	t.Logf("✅ Auth OK — session token: %s...%s", sessionToken[:20], sessionToken[len(sessionToken)-10:])

	// Init Noise IK client keypair
	clientKP, err = corecrypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate client keypair: %v", err)
	}
	serverPubKey = mustDecodeHex(serverPubKeyHex)
	_ = mustDecodeHex(noisePrivKeyHex)
	t.Log("✅ Noise IK client keypair generated")
}

// ─── Phase 2: Manifests ─────────────────────────────────────────────

func TestManifests(t *testing.T) {
	if sessionToken == "" {
		t.Skip("no session token — run TestAuth first")
	}
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v2/bot/%s/command/manifests", baseURL, botID), nil)
	req.Header.Set("X-Haruki-Bot-Id", botID)
	req.Header.Set("X-Haruki-Bot-Session-Token", sessionToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("manifest request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("manifest failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var mf struct {
		Data struct {
			Entries []struct {
				CommandPrefixes []string `json:"command_prefixes"`
				CommandPath     string   `json:"command_path"`
			} `json:"entries"`
		} `json:"data"`
	}
	json.Unmarshal(body, &mf)
	t.Logf("✅ Manifests OK — %d command groups", len(mf.Data.Entries))
	for _, e := range mf.Data.Entries {
		t.Logf("   %s → %s", e.CommandPrefixes, e.CommandPath)
	}
}

// ─── Phase 3: Bot Command Tests ─────────────────────────────────────

type cmdTest struct {
	name    string
	path    string
	cmd     string
	text    string
	wantOK  bool
}

func TestBotCommands(t *testing.T) {
	if sessionToken == "" {
		t.Skip("no session token — run TestAuth first")
	}

	// Prerequisite: bind account first
	t.Run("0-bind", func(t *testing.T) {
		data, status := sendBotCommand(t, "profile/bind", "/绑定", "/绑定 GAME_USER_ID_REDACTED")
		r := parseBotResp(t, data)
		if status == 200 && r.Message == "ok" {
			t.Log("✅ Bind OK")
		} else {
			t.Logf("⚠️  Bind: HTTP %d msg=%s", status, r.Message)
		}
	})

	tests := []cmdTest{
		// ─── Existing (round 1-4) ───────────────────────────────

		// Card
		{"card/detail", "card/detail", "/查卡", "/查卡 1", true},
		{"card/list", "card/list", "/查牌", "/查牌 初音未来", true},
		{"card/box", "card/box", "/查箱", "/查箱", true},

		// Music — basic
		{"music", "music", "/查曲", "/查曲 Tell Your World", true},

		// Event
		{"event", "event", "/查活动", "/查活动 1", true},
		{"event/list", "event/list", "/活动列表", "/活动列表", true},

		// Gacha
		{"gacha", "gacha", "/查卡池", "/查卡池 1", true},

		// Education
		{"education/challenge", "education/challenge", "/挑战信息", "/挑战信息", true},
		{"education/area", "education/area", "/区域道具", "/区域道具", true},
		{"education/bonds", "education/bonds", "/羁绊", "/羁绊", true},
		{"education/leader", "education/leader", "/领队统计", "/领队统计", true},
		{"education/power", "education/power", "/加成信息", "/加成信息", true},

		// Profile — basic
		{"profile", "profile", "/profile", "/profile", true},
		{"profile/reg-time", "profile/reg-time", "/注册时间", "/注册时间", true},

		// Stamp
		{"stamp", "stamp", "/查贴纸", "/查贴纸 1 2 3 4 5", true},

		// SK — basic
		{"sk/line", "sk/line", "/榜线", "/榜线 100", true},
		{"sk/query", "sk/query", "/sk查分", "/sk查分", true},

		// Score
		{"score/music-meta", "score/music-meta", "/歌曲meta", "/歌曲meta Tell Your World", true},

		// VLive
		{"vlive", "vlive", "/vlive", "/vlive", true},

		// Misc
		{"misc/birthday", "misc/birthday", "/生日", "/生日 miku", true},

		// Arrest
		{"arrest", "arrest", "/逮捕", "/逮捕", true},

		// ─── A: No user data needed ─────────────────────────────

		// Music sub-commands
		{"music/list", "music/list", "/歌曲列表", "/歌曲列表", true},
		{"music/bpm", "music/bpm", "/pjsk bpm", "/pjsk bpm Tell Your World", true},
		{"music/cover", "music/cover", "/pjsk music cover", "/pjsk music cover Tell Your World", true},
		{"music/note-count", "music/note-count", "/查物量", "/查物量 1000", true},
		{"music/rewards", "music/rewards", "/曲目奖励", "/曲目奖励 Tell Your World", true},

		// Score — music board (public ranking)
		{"score/music-board", "score/music-board", "/歌曲排行", "/歌曲排行 Tell Your World", true},

		// Alias — read-only
		{"alias/pending", "alias/pending", "/待审核别名", "/待审核别名", true},

		// Profile — read-only data checks
		{"profile/check-data", "profile/check-data", "/抓包数据", "/抓包数据", true},
		{"profile/check-data-mysekai", "profile/check-data-mysekai", "/msd", "/msd", true},
		{"profile/verify/list", "profile/verify/list", "/pjsk验证列表", "/pjsk验证列表", true},

		// ─── B: Profile reversible write ops (hide→show pairs) ──

		{"profile/suite/hide", "profile/suite/hide", "/pjsk隐藏抓包", "/pjsk隐藏抓包", true},
		{"profile/suite/show", "profile/suite/show", "/pjsk显示抓包", "/pjsk显示抓包", true},
		{"profile/mysekai/hide", "profile/mysekai/hide", "/pjsk隐藏烤森抓包", "/pjsk隐藏烤森抓包", true},
		{"profile/mysekai/show", "profile/mysekai/show", "/pjsk显示烤森抓包", "/pjsk显示烤森抓包", true},
		{"profile/visibility/hide", "profile/visibility/hide", "/pjsk hide id", "/pjsk hide id", true},
		{"profile/visibility/show", "profile/visibility/show", "/pjsk show id", "/pjsk show id", true},

		// ─── C: Needs Toolbox user suite data ───────────────────

		// Music — user progress
		{"music/progress", "music/progress", "/打歌进度", "/打歌进度", true},

		// Event record
		{"event/record", "event/record", "/活动记录", "/活动记录", true},

		// Deck
		{"deck/event", "deck/event", "/活动组卡", "/活动组卡", true},
		{"deck/challenge", "deck/challenge", "/挑战组卡", "/挑战组卡", true},
		{"deck/no-event", "deck/no-event", "/长草组卡", "/长草组卡", true},
		{"deck/bonus", "deck/bonus", "/加成组卡", "/加成组卡", true},
		{"deck/mysekai", "deck/mysekai", "/烤森组卡", "/烤森组卡", true},

		// Score — user score
		{"score", "score", "/控分", "/控分 100000 Tell Your World", true},
		{"score/custom-room", "score/custom-room", "/自定义房间控分", "/自定义房间控分 100000", true},

		// MySEKAI
		{"mysekai/resource", "mysekai/resource", "/mysekai资源", "/mysekai资源", true},
		{"mysekai/talk-list", "mysekai/talk-list", "/mysekai对话列表", "/mysekai对话列表", true},
		{"mysekai/fixture-list", "mysekai/fixture-list", "/mysekai家具列表", "/mysekai家具列表", true},
		{"mysekai/fixture-detail", "mysekai/fixture-detail", "/msf", "/msf 1", true},
		{"mysekai/door-upgrade", "mysekai/door-upgrade", "/mysekai大门升级", "/mysekai大门升级", true},
		{"mysekai/music-record", "mysekai/music-record", "/mysekai唱片", "/mysekai唱片", true},
		{"mysekai/photo", "mysekai/photo", "/msp", "/msp 1", true},

		// ─── D: SK Tracker realtime ─────────────────────────────

		{"sk/speed", "sk/speed", "/时速", "/时速", true},
		{"sk/check-room", "sk/check-room", "/sk查房", "/sk查房", true},
		{"sk/player-trace", "sk/player-trace", "/玩家轨迹", "/玩家轨迹", true},
		{"sk/rank-trace", "sk/rank-trace", "/档线轨迹", "/档线轨迹 100", true},
		{"sk/winrate", "sk/winrate", "/胜率预测", "/胜率预测", true},
	}

	results := make(map[string]string)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, status := sendBotCommand(t, tt.path, tt.cmd, tt.text)
			r := parseBotResp(t, data)

			if status != 200 {
				r2 := parseBotResp(t, data)
				errMsg := r2.Error
				if errMsg == "" {
					errMsg = truncate(data, 200)
				}
				t.Logf("⚠️  %s: HTTP %d — %s", tt.name, status, errMsg)
				results[tt.name] = fmt.Sprintf("HTTP %d: %s", status, truncate([]byte(errMsg), 80))
				return
			}

			if r.Error != "" {
				t.Logf("⚠️  %s: error=%s", tt.name, r.Error)
				results[tt.name] = fmt.Sprintf("error: %s", r.Error)
				return
			}

			if r.Message == "ok" && r.Data != nil {
				t.Logf("✅ %s: OK (data: %v)", tt.name, summarizeData(r.Data))
				results[tt.name] = "✅ OK"
			} else if r.Message == "ok" {
				t.Logf("✅ %s: OK (text-only response)", tt.name)
				results[tt.name] = "✅ OK (text)"
			} else {
				t.Logf("⚠️  %s: msg=%s data=%v", tt.name, r.Message, r.Data)
				results[tt.name] = fmt.Sprintf("msg=%s", r.Message)
			}
		})
	}

	t.Log("\n=== Results Summary ===")
	for name, result := range results {
		t.Logf("  %-30s %s", name, result)
	}
}

// ─── Phase 4: External API Proxy Tests ──────────────────────────────

func TestExternalAPIs(t *testing.T) {
	sekaiToken := os.Getenv("HARUKI_TEST_SEKAI_TOKEN")
	toolboxToken := os.Getenv("HARUKI_TEST_TOOLBOX_TOKEN")
	trackerBase := os.Getenv("HARUKI_TEST_TRACKER_BASE")
	sekaiBase := os.Getenv("HARUKI_TEST_SEKAI_BASE")
	toolboxBase := os.Getenv("HARUKI_TEST_TOOLBOX_BASE")

	if sekaiToken == "" || toolboxToken == "" || sekaiBase == "" || toolboxBase == "" || trackerBase == "" {
		t.Skip("external API env vars not set (HARUKI_TEST_SEKAI_TOKEN, HARUKI_TEST_TOOLBOX_TOKEN, HARUKI_TEST_SEKAI_BASE, HARUKI_TEST_TOOLBOX_BASE, HARUKI_TEST_TRACKER_BASE)")
	}

	t.Log("=== Phase 4: External API Proxy Tests ===")

	// Sekai API — profile
	t.Run("sekai-api/profile", func(t *testing.T) {
		url := fmt.Sprintf("%s/v6/api/jp/%s/profile", sekaiBase, gameUserID)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("X-Haruki-Sekai-Token", sekaiToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("sekai api: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		t.Logf("Sekai Profile: HTTP %d (%d bytes)", resp.StatusCode, len(body))
		if resp.StatusCode == 200 {
			t.Log("✅ Sekai API profile OK")
		} else {
			t.Logf("⚠️  Sekai API profile: %s", string(body[:min(len(body), 200)]))
		}
	})

	// Toolbox API — MySEKAI
	t.Run("toolbox/mysekai", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/private/game-data/jp/mysekai/%s?platform=qq&platform_user_id=%s", toolboxBase, gameUserID, platformUserID)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+toolboxToken)
		req.Header.Set("User-Agent", "Haruki-Cloud/v2.0.0")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("toolbox: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		t.Logf("Toolbox MySEKAI: HTTP %d (%d bytes)", resp.StatusCode, len(body))
		if resp.StatusCode == 200 {
			t.Log("✅ Toolbox MySEKAI OK")
		}
	})

	// Tracker API — event ranking
	t.Run("tracker/event-ranking", func(t *testing.T) {
		url := fmt.Sprintf("%s/event/jp/199/latest-ranking/rank/1", trackerBase)
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("tracker: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		t.Logf("Tracker Event: HTTP %d (%d bytes)", resp.StatusCode, len(body))
		if resp.StatusCode == 200 {
			t.Log("✅ Tracker Event Ranking OK")
		}
	})
}

func TestMain(m *testing.M) {
	// Ensure crypto seed
	buf := make([]byte, 1)
	rand.Read(buf)
	os.Exit(m.Run())
}
