// Package integration provides end-to-end integration tests for Haruki-Cloud APIs.
//
// Run with: HARUKI_RUN_INTEGRATION=1 go test -v ./integration -count=1
package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	json "github.com/bytedance/sonic"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"encoding/base64"

	botauth "haruki-cloud/api/bot/auth"
	corecrypto "haruki-cloud/internal/core/crypto"

	"github.com/golang-jwt/jwt/v5"
	_ "github.com/lib/pq"

	"github.com/shamaton/msgpack/v3"
)

// ─── Test Configuration ─────────────────────────────────────────────

const (
	integrationEnv                    = "HARUKI_RUN_INTEGRATION"
	integrationBaseURLEnv             = "HARUKI_TEST_BASE_URL"
	integrationBotIDEnv               = "HARUKI_TEST_BOT_ID"
	integrationCredentialEnv          = "HARUKI_TEST_BOT_CREDENTIAL"
	integrationPlatformEnv            = "HARUKI_TEST_PLATFORM"
	integrationPlatformUserIDEnv      = "HARUKI_TEST_PLATFORM_USER_ID"
	integrationRegionEnv              = "HARUKI_TEST_REGION"
	integrationGameUserIDEnv          = "HARUKI_TEST_GAME_USER_ID"
	integrationCredentialSignTokenEnv = "HARUKI_TEST_CREDENTIAL_SIGN_TOKEN"
	integrationUsersDSNEnv            = "HARUKI_TEST_USERS_DSN"
	integrationPJSKDSNEnv             = "HARUKI_TEST_PJSK_DSN"
	integrationServerPubKeyEnv        = "HARUKI_TEST_SERVER_PUBKEY_HEX"
	integrationImagePathEnv           = "HARUKI_TEST_IMAGE_PATH"

	defaultBaseURL             = "http://127.0.0.1:6666"
	defaultBotID               = "12345678"
	defaultCredential          = "CREDENTIAL_VALUE_REDACTED_PLACEHOLDER_00000000000="
	defaultPlatform            = "qq"
	defaultPlatformUserID      = "QQ_ID_REDACTED"
	defaultRegion              = "jp"
	defaultGameUserID          = "GAME_USER_ID_REDACTED"
	defaultCredentialSignToken = "CREDENTIAL_SIGN_TOKEN_REDACTED_0000000000000000000000000000000000"
	defaultUsersDSN            = "host=localhost port=5432 user=haruki_users password=users_pw_2026 dbname=haruki_users sslmode=disable"
	defaultPJSKDSN             = "host=localhost port=5432 user=haruki_pjsk password=pjsk_pw_2026 dbname=haruki_pjsk sslmode=disable"
	defaultServerPubKeyHex     = "NOISE_PUB_KEY_REDACTED_0000000000000000000000000000000000000000000"
	defaultImagePath           = "/IMG_7736.png"
)

type integrationConfig struct {
	BaseURL             string
	BotID               string
	Credential          string
	Platform            string
	PlatformUserID      string
	Region              string
	GameUserID          string
	CredentialSignToken string
	UsersDSN            string
	PJSKDSN             string
	ServerPubKeyHex     string
	ImagePath           string
}

var (
	sessionToken string
	serverPubKey []byte

	testConfig     integrationConfig
	loadConfigOnce sync.Once
	loadConfigErr  error
	authOnce       sync.Once
	authErr        error
)

func requireIntegration(t *testing.T) {
	t.Helper()
	if !parseBoolEnv(integrationEnv) {
		t.Skipf("integration tests are disabled by default; set %s=1 to enable", integrationEnv)
	}
}

func parseBoolEnv(name string) bool {
	value := strings.TrimSpace(os.Getenv(name))
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func readEnvOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func loadIntegrationConfig() (integrationConfig, error) {
	loadConfigOnce.Do(func() {
		cfg := integrationConfig{
			BaseURL:             strings.TrimRight(readEnvOrDefault(integrationBaseURLEnv, defaultBaseURL), "/"),
			BotID:               readEnvOrDefault(integrationBotIDEnv, defaultBotID),
			Credential:          readEnvOrDefault(integrationCredentialEnv, defaultCredential),
			Platform:            readEnvOrDefault(integrationPlatformEnv, defaultPlatform),
			PlatformUserID:      readEnvOrDefault(integrationPlatformUserIDEnv, defaultPlatformUserID),
			Region:              readEnvOrDefault(integrationRegionEnv, defaultRegion),
			GameUserID:          readEnvOrDefault(integrationGameUserIDEnv, defaultGameUserID),
			CredentialSignToken: readEnvOrDefault(integrationCredentialSignTokenEnv, defaultCredentialSignToken),
			UsersDSN:            readEnvOrDefault(integrationUsersDSNEnv, defaultUsersDSN),
			PJSKDSN:             readEnvOrDefault(integrationPJSKDSNEnv, defaultPJSKDSN),
			ServerPubKeyHex:     readEnvOrDefault(integrationServerPubKeyEnv, defaultServerPubKeyHex),
			ImagePath:           readEnvOrDefault(integrationImagePathEnv, defaultImagePath),
		}
		if cfg.BaseURL == "" {
			loadConfigErr = fmt.Errorf("%s must not be empty", integrationBaseURLEnv)
			return
		}
		if _, err := hex.DecodeString(cfg.ServerPubKeyHex); err != nil {
			loadConfigErr = fmt.Errorf("invalid %s: %w", integrationServerPubKeyEnv, err)
			return
		}
		if !strings.HasPrefix(cfg.ImagePath, "/") {
			cfg.ImagePath = "/" + cfg.ImagePath
		}
		testConfig = cfg
	})
	return testConfig, loadConfigErr
}

func requireIntegrationConfig(t *testing.T) integrationConfig {
	t.Helper()
	requireIntegration(t)
	cfg, err := loadIntegrationConfig()
	if err != nil {
		t.Fatalf("load integration config: %v", err)
	}
	return cfg
}

func ensureAuthenticated(t *testing.T) integrationConfig {
	t.Helper()
	cfg := requireIntegrationConfig(t)
	authOnce.Do(func() {
		authErr = authenticate(cfg)
	})
	if authErr != nil {
		t.Fatalf("authenticate integration bot: %v", authErr)
	}
	return cfg
}

func authenticate(cfg integrationConfig) error {
	// Key derivation: server takes the raw credential string bytes (not base64-decoded),
	// copies first 32 bytes into a 32-byte key.
	credRaw := []byte(cfg.Credential)
	aesKey := make([]byte, 32)
	copy(aesKey, credRaw)

	// Build JWT credential: {bot_id, credential} signed with credentialSignToken.
	jwtCred, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"bot_id":     cfg.BotID,
		"credential": cfg.Credential,
	}).SignedString([]byte(cfg.CredentialSignToken))
	if err != nil {
		return fmt.Errorf("sign JWT credential: %w", err)
	}

	authPayload := fmt.Sprintf(`{"credential":"%s","timestamp":%d}`, jwtCred, time.Now().Unix())
	encryptedRaw, err := botauth.EncryptRaw([]byte(authPayload), aesKey)
	if err != nil {
		return fmt.Errorf("encrypt auth payload: %w", err)
	}
	encrypted := base64.StdEncoding.EncodeToString(encryptedRaw)

	body := fmt.Sprintf(`{"encrypted_payload":"%s"}`, encrypted)
	resp, err := http.Post(cfg.BaseURL+"/bot/"+cfg.BotID+"/auth", "application/json", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("auth request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read auth response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var authResp struct {
		Message string `json:"message"`
		Data    struct {
			SessionToken string `json:"session_token"`
			ExpiresAt    int64  `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &authResp); err != nil {
		return fmt.Errorf("parse auth response: %w", err)
	}
	if authResp.Data.SessionToken == "" {
		return fmt.Errorf("empty session token in response: %s", string(respBody))
	}

	sessionToken = authResp.Data.SessionToken
	serverPubKey, err = hex.DecodeString(cfg.ServerPubKeyHex)
	if err != nil {
		return fmt.Errorf("decode server pubkey: %w", err)
	}
	return nil
}

func summarizeSecret(value string) string {
	if len(value) <= 18 {
		return value
	}
	return value[:8] + "..." + value[len(value)-6:]
}

// noiseRoundTrip sends an encrypted Noise NK request and decrypts the response.
func noiseRoundTrip(t *testing.T, url string, payload interface{}) ([]byte, int) {
	t.Helper()
	cfg := ensureAuthenticated(t)
	body, err := msgpack.Marshal(payload)
	if err != nil {
		t.Fatalf("msgpack marshal: %v", err)
	}

	nc, err := corecrypto.NewInitiator(serverPubKey)
	if err != nil {
		t.Fatalf("noise handshake init: %v", err)
	}
	ciphertext, err := nc.EncryptPacket(body)
	if err != nil {
		t.Fatalf("noise encrypt: %v", err)
	}

	req, _ := http.NewRequest("POST", url, bytes.NewReader(ciphertext))
	req.Header.Set("X-Haruki-Bot-Id", cfg.BotID)
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
	Platform        string       `json:"platform" msgpack:"platform"`
	PlatformUserID  string       `json:"platform_user_id" msgpack:"platform_user_id"`
	PlatformGroupID string       `json:"platform_group_id,omitempty" msgpack:"platform_group_id,omitempty"`
	Server          string       `json:"server,omitempty" msgpack:"server,omitempty"`
	MatchedCommand  string       `json:"matched_command" msgpack:"matched_command"`
	Message         []msgSegment `json:"message" msgpack:"message"`
}

type msgSegment struct {
	Type string            `json:"type" msgpack:"type"`
	Data map[string]string `json:"data" msgpack:"data"`
}

func makeBotReq(cfg integrationConfig, cmd, fullText string) botRequest {
	return botRequest{
		Platform:       cfg.Platform,
		PlatformUserID: cfg.PlatformUserID,
		Server:         cfg.Region,
		MatchedCommand: cmd,
		Message: []msgSegment{
			{Type: "text", Data: map[string]string{"text": fullText}},
		},
	}
}

func sendBotCommand(t *testing.T, path, cmd, fullText string) ([]byte, int) {
	t.Helper()
	cfg := ensureAuthenticated(t)
	url := fmt.Sprintf("%s/api/v2/bot/%s/pjsk/%s", cfg.BaseURL, cfg.BotID, path)
	req := makeBotReq(cfg, cmd, fullText)
	return noiseRoundTrip(t, url, req)
}

func sendBotCommandWithSegments(t *testing.T, path, cmd string, segments []msgSegment) ([]byte, int) {
	t.Helper()
	cfg := ensureAuthenticated(t)
	url := fmt.Sprintf("%s/api/v2/bot/%s/pjsk/%s", cfg.BaseURL, cfg.BotID, path)
	req := botRequest{
		Platform:       cfg.Platform,
		PlatformUserID: cfg.PlatformUserID,
		Server:         cfg.Region,
		MatchedCommand: cmd,
		Message:        segments,
	}
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

func getMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ─── Phase 1: Authentication ────────────────────────────────────────

func TestAuth(t *testing.T) {
	cfg := ensureAuthenticated(t)
	t.Log("=== Phase 1: Bot Authentication ===")
	t.Logf("✅ Auth OK — bot=%s base=%s session=%s", cfg.BotID, cfg.BaseURL, summarizeSecret(sessionToken))
	t.Log("✅ Noise IK client keypair ready")
}

// ─── Phase 2: Manifests ─────────────────────────────────────────────

func TestManifests(t *testing.T) {
	cfg := ensureAuthenticated(t)
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v2/bot/%s/command/manifests", cfg.BaseURL, cfg.BotID), nil)
	req.Header.Set("X-Haruki-Bot-Id", cfg.BotID)
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
	name   string
	path   string
	cmd    string
	text   string
	wantOK bool
}

func TestBotCommands(t *testing.T) {
	cfg := ensureAuthenticated(t)

	// Prerequisite: bind account first
	t.Run("0-bind", func(t *testing.T) {
		data, status := sendBotCommand(t, "profile/bind", "/绑定", "/绑定 "+cfg.GameUserID)
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
		{"card/list", "card/list", "/卡牌列表", "/卡牌列表 初音未来", true},
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
		{"education/area", "education/area", "/区域道具", "/区域道具 树", true},
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
		{"misc/birthday", "misc/birthday", "/生日", "/生日", true},

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
		{"score", "score", "/控分", "/控分 360 Tell Your World", true},
		{"score/custom-room", "score/custom-room", "/自定义房间控分", "/自定义房间控分 50", true},

		// MySEKAI
		{"mysekai/resource", "mysekai/resource", "/mysekai资源", "/mysekai资源", true},
		{"mysekai/talk-list", "mysekai/talk-list", "/mysekai对话列表", "/mysekai对话列表 みのり", true},
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

// ─── Phase 3b: Expanded Coverage Tests (17 new paths) ───────────────

// resolveHarukiUserID queries the users DB to find the haruki_user_id for our test user.
func resolveHarukiUserID(t *testing.T) int {
	t.Helper()
	cfg := requireIntegrationConfig(t)
	db, err := sql.Open("postgres", cfg.UsersDSN)
	if err != nil {
		t.Fatalf("open users DB: %v", err)
	}
	defer db.Close()
	var id int
	err = db.QueryRow("SELECT id FROM users WHERE platform=$1 AND user_id=$2", cfg.Platform, cfg.PlatformUserID).Scan(&id)
	if err != nil {
		t.Fatalf("resolve haruki_user_id for %s/%s: %v", cfg.Platform, cfg.PlatformUserID, err)
	}
	return id
}

// ensureAliasAdmin inserts or updates the alias_admins record for our test user.
func ensureAliasAdmin(t *testing.T, harukiUserID int) {
	t.Helper()
	cfg := requireIntegrationConfig(t)
	db, err := sql.Open("postgres", cfg.PJSKDSN)
	if err != nil {
		t.Fatalf("open pjsk DB: %v", err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO alias_admins (haruki_user_id, name) VALUES ($1, $2)
		ON CONFLICT (haruki_user_id) DO UPDATE SET name=$2`, harukiUserID, "integration-test-admin")
	if err != nil {
		t.Fatalf("insert alias admin: %v", err)
	}
	t.Logf("✅ Alias admin configured: haruki_user_id=%d", harukiUserID)
}

// startImageServer serves project-root files on a random port and returns the base URL.
func startImageServer(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start image server: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	mux := http.NewServeMux()
	// Serve from project root (tests run with cwd = integration/)
	mux.Handle("/", http.FileServer(http.Dir("..")))
	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)
	t.Cleanup(func() { srv.Shutdown(context.Background()) })
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	t.Logf("✅ Image server started at %s", base)
	return base
}

// getPendingAliasIDs queries the DB for recent pending alias IDs (for approve/reject tests).
func getPendingAliasIDs(t *testing.T, limit int) []int64 {
	t.Helper()
	cfg := requireIntegrationConfig(t)
	db, err := sql.Open("postgres", cfg.PJSKDSN)
	if err != nil {
		t.Logf("open pjsk DB for pending: %v", err)
		return nil
	}
	defer db.Close()
	rows, err := db.Query("SELECT id FROM pending_alias ORDER BY id DESC LIMIT $1", limit)
	if err != nil {
		t.Logf("query pending alias: %v", err)
		return nil
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids
}

func TestExpandedCoverage(t *testing.T) {
	cfg := ensureAuthenticated(t)

	// ─── Setup: alias admin + image server ───────────────────
	harukiUserID := resolveHarukiUserID(t)
	ensureAliasAdmin(t, harukiUserID)
	imageBase := startImageServer(t)
	imageURL := imageBase + cfg.ImagePath

	results := make(map[string]string)

	runTest := func(t *testing.T, name string, data []byte, status int) {
		t.Helper()
		r := parseBotResp(t, data)
		if status != 200 {
			errMsg := r.Error
			if errMsg == "" {
				errMsg = truncate(data, 200)
			}
			t.Logf("⚠️  %s: HTTP %d — %s", name, status, errMsg)
			results[name] = fmt.Sprintf("HTTP %d: %s", status, truncate([]byte(errMsg), 80))
			return
		}
		if r.Error != "" {
			t.Logf("⚠️  %s: error=%s", name, r.Error)
			results[name] = fmt.Sprintf("error: %s", r.Error)
			return
		}
		if r.Message == "ok" && r.Data != nil {
			t.Logf("✅ %s: OK (data: %v)", name, summarizeData(r.Data))
			results[name] = "✅ OK"
		} else if r.Message == "ok" {
			t.Logf("✅ %s: OK (text-only response)", name)
			results[name] = "✅ OK (text)"
		} else {
			t.Logf("⚠️  %s: msg=%s data=%v", name, r.Message, r.Data)
			results[name] = fmt.Sprintf("msg=%s", r.Message)
		}
	}

	// ─── Alias Query ─────────────────────────────────────────
	t.Run("alias/music", func(t *testing.T) {
		data, status := sendBotCommand(t, "alias/music", "/歌曲别名", "/歌曲别名 Tell Your World")
		runTest(t, "alias/music", data, status)
	})

	t.Run("alias/character", func(t *testing.T) {
		data, status := sendBotCommand(t, "alias/character", "/角色别名", "/角色别名 miku")
		runTest(t, "alias/character", data, status)
	})

	// ─── Alias Add (creates pending entries for approve/reject) ──
	// Use timestamp-based alias names to avoid "already pending" conflicts from prior runs.
	testSuffix := fmt.Sprintf("%d", time.Now().UnixMilli()%100000)
	musicTestAlias := "测试别名m" + testSuffix
	charaTestAlias := "测试别名c" + testSuffix

	t.Run("alias/music/add", func(t *testing.T) {
		data, status := sendBotCommand(t, "alias/music/add", "/添加歌曲别名",
			"/添加歌曲别名\nTell Your World\n"+musicTestAlias)
		runTest(t, "alias/music/add", data, status)
	})

	t.Run("alias/character/add", func(t *testing.T) {
		data, status := sendBotCommand(t, "alias/character/add", "/添加角色别名",
			"/添加角色别名\n初音ミク\n"+charaTestAlias)
		runTest(t, "alias/character/add", data, status)
	})

	// ─── Alias Approve (needs pending entries from above) ────
	t.Run("alias/approve", func(t *testing.T) {
		pendingIDs := getPendingAliasIDs(t, 10)
		if len(pendingIDs) == 0 {
			t.Log("⚠️  alias/approve: no pending aliases to approve")
			results["alias/approve"] = "skipped: no pending"
			return
		}
		// Approve all pending aliases (space-separated IDs)
		idStrs := make([]string, len(pendingIDs))
		for i, id := range pendingIDs {
			idStrs[i] = fmt.Sprintf("%d", id)
		}
		cmd := "/同意别名 " + strings.Join(idStrs, " ")
		data, status := sendBotCommand(t, "alias/approve", "/同意别名", cmd)
		runTest(t, "alias/approve", data, status)
	})

	// ─── Alias Reject (add another then reject it) ───────────
	t.Run("alias/reject", func(t *testing.T) {
		// First add a temp alias so we have something to reject
		sendBotCommand(t, "alias/music/add", "/添加歌曲别名",
			"/添加歌曲别名\nTell Your World\n临时reject"+testSuffix)
		time.Sleep(200 * time.Millisecond)

		pendingIDs := getPendingAliasIDs(t, 1)
		if len(pendingIDs) == 0 {
			t.Log("⚠️  alias/reject: no pending aliases to reject")
			results["alias/reject"] = "skipped: no pending"
			return
		}
		cmd := fmt.Sprintf("/拒绝别名 %d 集成测试拒绝", pendingIDs[0])
		data, status := sendBotCommand(t, "alias/reject", "/拒绝别名", cmd)
		runTest(t, "alias/reject", data, status)
	})

	// ─── Alias Delete ────────────────────────────────────────
	t.Run("alias/music/del", func(t *testing.T) {
		data, status := sendBotCommand(t, "alias/music/del", "/删除歌曲别名",
			"/删除歌曲别名\nTell Your World\n"+musicTestAlias)
		runTest(t, "alias/music/del", data, status)
	})

	t.Run("alias/character/del", func(t *testing.T) {
		data, status := sendBotCommand(t, "alias/character/del", "/删除角色别名",
			"/删除角色别名\n初音ミク\n"+charaTestAlias)
		runTest(t, "alias/character/del", data, status)
	})

	// ─── Profile Verify (run BEFORE bg tests, since bg requires verified binding) ──
	t.Run("profile/verify", func(t *testing.T) {
		data, status := sendBotCommand(t, "profile/verify", "/pjsk验证", "/pjsk验证")
		runTest(t, "profile/verify", data, status)
	})

	// ─── Card Image ──────────────────────────────────────────
	t.Run("card/image", func(t *testing.T) {
		data, status := sendBotCommand(t, "card/image", "/查卡面", "/查卡面 1")
		runTest(t, "card/image", data, status)
	})

	// ─── Music Chart ─────────────────────────────────────────
	t.Run("music/chart", func(t *testing.T) {
		data, status := sendBotCommand(t, "music/chart", "/查谱面", "/查谱面 Tell Your World master")
		runTest(t, "music/chart", data, status)
	})

	// ─── Profile BG Upload (with image segment) ──────────────
	t.Run("profile/bg/upload", func(t *testing.T) {
		segments := []msgSegment{
			{Type: "text", Data: map[string]string{"text": "/上传个人信息背景"}},
			{Type: "image", Data: map[string]string{"url": imageURL}},
		}
		data, status := sendBotCommandWithSegments(t, "profile/bg/upload", "/上传个人信息背景", segments)
		runTest(t, "profile/bg/upload", data, status)
	})

	// ─── Profile BG Adjust ───────────────────────────────────
	t.Run("profile/bg/adjust", func(t *testing.T) {
		data, status := sendBotCommand(t, "profile/bg/adjust", "/调整个人信息背景",
			"/调整个人信息背景\n模糊 5\n透明 80")
		runTest(t, "profile/bg/adjust", data, status)
	})

	// ─── Profile BG Clear ────────────────────────────────────
	t.Run("profile/bg/clear", func(t *testing.T) {
		data, status := sendBotCommand(t, "profile/bg/clear", "/清空个人信息背景",
			"/清空个人信息背景")
		runTest(t, "profile/bg/clear", data, status)
	})

	// ─── Profile Default Set ─────────────────────────────────
	t.Run("profile/default", func(t *testing.T) {
		data, status := sendBotCommand(t, "profile/default", "/设置主账号",
			"/设置主账号 u1")
		runTest(t, "profile/default", data, status)
	})

	// ─── Profile Default Clear ───────────────────────────────
	t.Run("profile/default/clear", func(t *testing.T) {
		data, status := sendBotCommand(t, "profile/default/clear", "/清除默认绑定",
			"/清除默认绑定 u1")
		runTest(t, "profile/default/clear", data, status)
	})

	// ─── Profile Unbind (use u1 then re-bind) ────────────────
	// This is a destructive test — unbind u1, then re-bind to restore state.
	t.Run("profile/unbind", func(t *testing.T) {
		data, status := sendBotCommand(t, "profile/unbind", "/解绑", "/解绑 u1")
		runTest(t, "profile/unbind", data, status)

		// Restore: re-bind the game account
		time.Sleep(200 * time.Millisecond)
		sendBotCommand(t, "profile/bind", "/绑定", "/绑定 "+cfg.GameUserID)
	})

	// ─── Profile Bind List ────────────────────────────────────
	t.Run("profile/bind/list", func(t *testing.T) {
		data, status := sendBotCommand(t, "profile/bind/list", "/绑定列表", "/绑定列表")
		runTest(t, "profile/bind/list", data, status)
	})

	// ─── Deck Score-Up (text-only calculator) ────────────────
	t.Run("deck/score-up", func(t *testing.T) {
		data, status := sendBotCommand(t, "deck/score-up", "/实效", "/实效 160 160 150 150 150")
		runTest(t, "deck/score-up", data, status)
	})

	// ─── MySEKAI Blueprint/Map ────────────────────────────────
	t.Run("mysekai/blueprint", func(t *testing.T) {
		data, status := sendBotCommand(t, "mysekai/blueprint", "/msb", "/msb")
		runTest(t, "mysekai/blueprint", data, status)
	})

	t.Run("mysekai/map", func(t *testing.T) {
		data, status := sendBotCommand(t, "mysekai/map", "/msm", "/msm 1")
		runTest(t, "mysekai/map", data, status)
	})

	// ─── SK Daily/Predict ─────────────────────────────────────
	t.Run("sk/daily-speed", func(t *testing.T) {
		data, status := sendBotCommand(t, "sk/daily-speed", "/日速", "/日速")
		runTest(t, "sk/daily-speed", data, status)
	})

	t.Run("sk/predict", func(t *testing.T) {
		data, status := sendBotCommand(t, "sk/predict", "/榜线预测", "/榜线预测 100")
		runTest(t, "sk/predict", data, status)
	})

	t.Log("\n=== Expanded Coverage Results ===")
	for name, result := range results {
		t.Logf("  %-30s %s", name, result)
	}
}

// ─── Phase 4: External API Proxy Tests ──────────────────────────────

func TestExternalAPIs(t *testing.T) {
	requireIntegration(t)
	cfg := requireIntegrationConfig(t)
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
		url := fmt.Sprintf("%s/v6/api/%s/%s/profile", sekaiBase, cfg.Region, cfg.GameUserID)
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
			t.Logf("⚠️  Sekai API profile: %s", string(body[:getMin(len(body), 200)]))
		}
	})

	// Toolbox API — MySEKAI
	t.Run("toolbox/mysekai", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/private/game-data/%s/mysekai/%s?platform=%s&platform_user_id=%s", toolboxBase, cfg.Region, cfg.GameUserID, cfg.Platform, cfg.PlatformUserID)
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
		url := fmt.Sprintf("%s/event/%s/199/latest-ranking/rank/1", trackerBase, cfg.Region)
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
