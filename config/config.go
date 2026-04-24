package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/core/upstream"

	"gopkg.in/yaml.v3"
)

// Profile represents the deployment environment.
type Profile string

const (
	ProfileProduction Profile = "production"
	ProfileBeta       Profile = "beta"
	ProfileDev        Profile = "dev"
)

// ================= Log Levels =================

const (
	LogLevelDebug = "DEBUG"
	LogLevelInfo  = "INFO"
	LogLevelWarn  = "WARN"
)

// ParseProfile normalises a raw string into a known Profile.
// Empty string defaults to "dev".
func ParseProfile(raw string) (Profile, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "production", "prod":
		return ProfileProduction, nil
	case "beta", "test", "staging":
		return ProfileBeta, nil
	case "dev", "development", "":
		return ProfileDev, nil
	default:
		return "", fmt.Errorf("unknown profile %q (must be production, beta, or dev)", raw)
	}
}

func (p Profile) IsProduction() bool { return p == ProfileProduction }
func (p Profile) IsBeta() bool       { return p == ProfileBeta }
func (p Profile) IsDev() bool        { return p == ProfileDev }

// envStr overrides dst with the value of the named env var if set and non-empty.
func envStr(name string, dst *string) {
	if v := os.Getenv(name); v != "" {
		*dst = v
	}
}

// envInt overrides dst with the integer value of the named env var if set and valid.
func envInt(name string, dst *int) {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*dst = n
		}
	}
}

// envBool overrides dst with the bool value of the named env var if set and valid ("true"/"1"/"false"/"0").
func envBool(name string, dst *bool) {
	if v := os.Getenv(name); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			*dst = b
		}
	}
}

// envDuration overrides dst with the duration value of the named env var if set and valid.
func envDuration(name string, dst *time.Duration) {
	if v := os.Getenv(name); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			*dst = d
		}
	}
}

// ApplyEnvOverrides replaces key config fields with environment variables when set.
// Env var names follow the pattern HARUKI_<SECTION>_<FIELD> (all upper-snake).
func ApplyEnvOverrides(cfg *Config) {
	// Profile (env override parsed into typed field)
	if v := os.Getenv("HARUKI_PROFILE"); v != "" {
		if p, err := ParseProfile(v); err == nil {
			cfg.Profile = p
		}
	}

	// Backend
	envStr("HARUKI_BACKEND_HOST", &cfg.Backend.Host)
	envInt("HARUKI_BACKEND_PORT", &cfg.Backend.Port)
	envBool("HARUKI_BACKEND_SSL", &cfg.Backend.SSL)
	envStr("HARUKI_BACKEND_SSL_CERT", &cfg.Backend.SSLCert)
	envStr("HARUKI_BACKEND_SSL_KEY", &cfg.Backend.SSLKey)
	envStr("HARUKI_BACKEND_LOG_LEVEL", &cfg.Backend.LogLevel)
	envStr("HARUKI_BACKEND_ACCEPT_AUTHORIZATION", &cfg.Backend.AcceptAuthorization)
	envStr("HARUKI_BACKEND_ACCEPT_USER_AGENT", &cfg.Backend.AcceptUserAgent)
	envBool("HARUKI_BACKEND_ALLOW_INSECURE_INTERNAL_API", &cfg.Backend.AllowInsecureInternalAPI)
	envStr("HARUKI_LATEST_CLIENT_VERSION", &cfg.Backend.LatestHarukiClientVersion)

	// Redis
	envStr("HARUKI_REDIS_HOST", &cfg.Redis.Host)
	envInt("HARUKI_REDIS_PORT", &cfg.Redis.Port)
	envStr("HARUKI_REDIS_PASSWORD", &cfg.Redis.Password)

	// PJSK
	envBool("HARUKI_PJSK_ENABLED", &cfg.PJSK.Enabled)
	envStr("HARUKI_PJSK_DB_TYPE", &cfg.PJSK.DBType)
	envStr("HARUKI_PJSK_DB_URL", &cfg.PJSK.DBURL)

	// Sekai
	envBool("HARUKI_SEKAI_ENABLED", &cfg.Sekai.Enabled)
	envStr("HARUKI_SEKAI_DB_TYPE", &cfg.Sekai.DBType)
	envStr("HARUKI_SEKAI_DB_URL", &cfg.Sekai.DBURL)

	// Chunithm
	envBool("HARUKI_CHUNITHM_ENABLED", &cfg.Chunithm.Enabled)
	envStr("HARUKI_CHUNITHM_MUSIC_DB_TYPE", &cfg.Chunithm.MusicDBType)
	envStr("HARUKI_CHUNITHM_MUSIC_DB_URL", &cfg.Chunithm.MusicDBURL)
	envStr("HARUKI_CHUNITHM_BINDING_DB_TYPE", &cfg.Chunithm.BindingDBType)
	envStr("HARUKI_CHUNITHM_BINDING_DB_URL", &cfg.Chunithm.BindingDBURL)

	// Users DB
	envStr("HARUKI_USERS_DB_TYPE", &cfg.UsersDB.DBType)
	envStr("HARUKI_USERS_DB_URL", &cfg.UsersDB.DBURL)

	// Haruki Bot
	envStr("HARUKI_BOT_DB_TYPE", &cfg.HarukiBotDB.DBType)
	envStr("HARUKI_BOT_DB_URL", &cfg.HarukiBotDB.DBURL)
	envStr("HARUKI_BOT_CREDENTIAL_SIGN_TOKEN", &cfg.HarukiBotDB.CredentialSignToken)
	envStr("HARUKI_BOT_SESSION_SIGN_TOKEN", &cfg.HarukiBotDB.SessionSignToken)
	envStr("HARUKI_BOT_INTERNAL_API_TOKEN", &cfg.HarukiBotDB.InternalAPIToken)
	envInt("HARUKI_BOT_SESSION_TTL_DAYS", &cfg.HarukiBotDB.SessionTTLDays)
	envStr("HARUKI_BOT_NOISE_PRIVATE_KEY", &cfg.HarukiBotDB.NoisePrivateKey)
	envStr("HARUKI_BOT_AUTH_ENCRYPTION_KEY", &cfg.HarukiBotDB.AuthEncryptionKey)

	// Sekai API
	envStr("HARUKI_SEKAI_API_BASE_URL", &cfg.SekaiAPI.BaseURL)
	envStr("HARUKI_SEKAI_API_TOKEN", &cfg.SekaiAPI.Token)

	// Toolbox
	envStr("HARUKI_TOOLBOX_BASE_URL", &cfg.Toolbox.BaseURL)
	envStr("HARUKI_TOOLBOX_API_TOKEN", &cfg.Toolbox.APIToken)
	envStr("HARUKI_TOOLBOX_USER_AGENT", &cfg.Toolbox.UserAgent)

	// Tracker
	envStr("HARUKI_TRACKER_BASE_URL", &cfg.Tracker.BaseURL)
	envStr("HARUKI_TRACKER_USER_AGENT", &cfg.Tracker.UserAgent)

	// Censor
	envStr("HARUKI_CENSOR_BAIDU_API_KEY", &cfg.Censor.BaiduAPIKey)
	envStr("HARUKI_CENSOR_BAIDU_SECRET", &cfg.Censor.BaiduSecret)
	envStr("HARUKI_CENSOR_TENCENT_SECRET_ID", &cfg.Censor.TencentSecretID)
	envStr("HARUKI_CENSOR_TENCENT_SECRET_KEY", &cfg.Censor.TencentSecretKey)
	envStr("HARUKI_CENSOR_TENCENT_REGION", &cfg.Censor.TencentRegion)
	envStr("HARUKI_CENSOR_TENCENT_BIZ_TYPE", &cfg.Censor.TencentBizType)
	envStr("HARUKI_CENSOR_DB_TYPE", &cfg.Censor.CensorDBType)
	envStr("HARUKI_CENSOR_DB_URL", &cfg.Censor.CensorDBURL)

	// PJSK Render
	envBool("HARUKI_PJSK_RENDER_ENABLED", &cfg.PJSKRender.Enabled)
	envStr("HARUKI_PJSK_RENDER_DRAWING_BASE_URL", &cfg.PJSKRender.DrawingBaseURL)
	envStr("HARUKI_PJSK_RENDER_IMAGE_CACHE_CHARTS_URI", &cfg.PJSKRender.ImageCache.ChartsURI)
	envStr("HARUKI_PJSK_RENDER_IMAGE_CACHE_PG_URL", &cfg.PJSKRender.ImageCache.PGURL)
	envStr("HARUKI_PJSK_RENDER_ASSETS_BASE_URL", &cfg.PJSKRender.AssetDirs.AssetsBaseURL)
	envDuration("HARUKI_PJSK_RENDER_MUSIC_META_REFRESH_INTERVAL", &cfg.PJSKRender.MusicMeta.RefreshInterval)
	envStr("HARUKI_PJSK_RENDER_DECK_RECOMMEND_SERVICE_BASE_URL", &cfg.PJSKRender.DeckRecommend.ServiceBaseURL)
	envStr("HARUKI_PJSK_RENDER_DECK_RECOMMEND_MASTERDATA_DIR", &cfg.PJSKRender.DeckRecommend.MasterdataDir)
	envBool("HARUKI_PJSK_RENDER_LOCAL_MASTERDATA_ALLOW_LEAKS", &cfg.PJSKRender.LocalMasterdata.AllowLeaks)
}

type BackendConfig struct {
	Host                     string        `yaml:"host"`
	Port                     int           `yaml:"port"`
	SSL                      bool          `yaml:"ssl"`
	SSLCert                  string        `yaml:"ssl_cert"`
	SSLKey                   string        `yaml:"ssl_key"`
	LogLevel                 string        `yaml:"log_level"`
	MainLogFile              string        `yaml:"main_log_file"`
	AccessLog                string        `yaml:"access_log"`
	APICacheTTL              time.Duration `yaml:"api_cache_ttl"`
	AccessLogPath            string        `yaml:"access_log_path"`
	AcceptAuthorization      string        `yaml:"accept_authorization"`
	AcceptUserAgent          string        `yaml:"accept_user_agent"`
	AllowInsecureInternalAPI bool          `yaml:"allow_insecure_internal_api"`
	EnableTrustProxy         bool          `yaml:"enable_trust_proxy"`
	TrustProxies             []string      `yaml:"trusted_proxies"`
	ProxyHeader              string        `yaml:"proxy_header"`
	LatestHarukiClientVersion string       `yaml:"latest_haruki_client_version"`
}

type ChunithmConfig struct {
	Enabled       bool   `yaml:"enabled"`
	MusicDBType   string `yaml:"music_db_type"`
	MusicDBURL    string `yaml:"music_db_url"`
	BindingDBType string `yaml:"binding_db_type"`
	BindingDBURL  string `yaml:"binding_db_url"`
}

type PJSKConfig struct {
	Enabled        bool                      `yaml:"enabled"`
	DBType         string                    `yaml:"db_type"`
	DBURL          string                    `yaml:"db_url"`
	AllowCNMySekai []MySekaiCNWhitelistEntry `yaml:"allow_cn_mysekai"`
}

type SekaiConfig struct {
	Enabled bool   `yaml:"enabled"`
	DBType  string `yaml:"db_type"`
	DBURL   string `yaml:"db_url"`
}

type AssetDirsConfig struct {
	Primary       string   `yaml:"primary"`
	Legacy        []string `yaml:"legacy"`
	AssetsBaseURL string   `yaml:"assets_base_url"` // CDN/static base URL for direct asset serving (no imagecache)
}

type LocalMasterdataConfig struct {
	Enabled       bool   `yaml:"enabled"`
	AllowFallback bool   `yaml:"allow_fallback"` // when false, DB failure is fatal (production); when true, fallback to local files (dev/test)
	AllowLeaks    bool   `yaml:"allow_leaks"`    // when true, unopened event/worldbloom deck queries may fall back to local masterdata
	Dir           string `yaml:"dir"`
}

type UserSnapshotConfig struct {
	Provider      string `yaml:"provider"`
	AllowFallback bool   `yaml:"allow_fallback"` // when false, Toolbox failure is fatal (production); when true, fallback to local snapshot (dev/test)
	UserJSON      string `yaml:"user_json"`
	MusicMetaJSON string `yaml:"music_meta_json"`
	MySekaiJSON   string `yaml:"mysekai_json"`
}

type DeckRecommendConfig struct {
	Enabled        bool                    `yaml:"enabled"`
	ServiceBaseURL string                  `yaml:"service_base_url"`
	Targets        []upstream.TargetConfig `yaml:"targets"`
	MasterdataDir  string                  `yaml:"masterdata_dir"`
	Timeout        time.Duration           `yaml:"timeout"`
	MaxRetries     int                     `yaml:"max_retries"`
	RetryWaitTime  time.Duration           `yaml:"retry_wait_time"`
	DefaultAlgs    []string                `yaml:"default_algs"`
}

type RenderCacheConfig struct {
	BaseURL    string        `yaml:"base_url"`
	StorageDir string        `yaml:"storage_dir"`
	TTL        time.Duration `yaml:"ttl"`
}

type ImageCacheConfig struct {
	URI       string `yaml:"uri"`
	ChartsURI string `yaml:"charts_uri"`
	Dir       string `yaml:"dir"`
	PGURL     string `yaml:"pg_url"` // PostgreSQL DSN for deduplication store (optional)
}

type MusicMetaConfig struct {
	RefreshInterval time.Duration `yaml:"refresh_interval"` // default: 30m
}

// MySekaiCNWhitelistEntry defines a platform+group pair allowed to use
// MySekai features on the CN region.
type MySekaiCNWhitelistEntry struct {
	Platform string `yaml:"platform"`
	GroupID  string `yaml:"group_id"`
	BotID    string `yaml:"bot_id"`
}

type PJSKRenderConfig struct {
	Enabled           bool                    `yaml:"enabled"`
	DrawingBaseURL    string                  `yaml:"drawing_base_url"`
	DrawingTargets    []upstream.TargetConfig `yaml:"drawing_targets"`
	DrawingTimeout    time.Duration           `yaml:"drawing_timeout"`
	DrawingRetryCount int                     `yaml:"drawing_retry_count"`
	DrawingCache      RenderCacheConfig       `yaml:"drawing_cache"`
	ImageCache        ImageCacheConfig        `yaml:"image_cache"`
	AssetDirs         AssetDirsConfig         `yaml:"asset_dirs"`
	LocalMasterdata   LocalMasterdataConfig   `yaml:"local_masterdata"`
	UserSnapshot      UserSnapshotConfig      `yaml:"user_snapshot"`
	MusicMeta         MusicMetaConfig         `yaml:"music_meta"`
	DeckRecommend     DeckRecommendConfig     `yaml:"deck_recommend"`
}

type CensorConfig struct {
	// Text censor — Baidu AI Content Censor (TextCensor)
	BaiduAPIKey string `yaml:"baidu_api_key"`
	BaiduSecret string `yaml:"baidu_secret"`
	// Image censor — Tencent Cloud Image Moderation Service (IMS)
	TencentSecretID  string `yaml:"tencent_secret_id"`
	TencentSecretKey string `yaml:"tencent_secret_key"`
	TencentRegion    string `yaml:"tencent_region"`   // default: ap-guangzhou
	TencentBizType   string `yaml:"tencent_biz_type"` // optional Biz type tag
	// Censor result database
	CensorDBType string `yaml:"censor_db_type"`
	CensorDBURL  string `yaml:"censor_db_url"`
}

type HarukiBotDBConfig struct {
	DBType              string `yaml:"db_type"`
	DBURL               string `yaml:"db_url"`
	CredentialSignToken string `yaml:"credential_sign_token"`
	SessionSignToken    string `yaml:"session_sign_token"`
	InternalAPIToken    string `yaml:"internal_api_token"`
	SessionTTLDays      int    `yaml:"session_ttl_days"`
	NoisePrivateKey     string `yaml:"noise_private_key"`
	AuthEncryptionKey   string `yaml:"auth_encryption_key"`
}

type UsersDBConfig struct {
	DBType string `yaml:"db_type"`
	DBURL  string `yaml:"db_url"`
}

type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
}

type SekaiAPIConfig struct {
	BaseURL string                  `yaml:"base_url"`
	Token   string                  `yaml:"token"`
	Targets []upstream.TargetConfig `yaml:"targets"`
}

type TrackerConfig struct {
	BaseURL   string `yaml:"base_url"`
	UserAgent string `yaml:"user_agent"`
}

type ToolboxConfig struct {
	BaseURL   string `yaml:"base_url"`
	APIToken  string `yaml:"api_token"`
	UserAgent string `yaml:"user_agent"`
}

type Config struct {
	Profile     Profile           `yaml:"profile"`
	Backend     BackendConfig     `yaml:"backend"`
	Chunithm    ChunithmConfig    `yaml:"chunithm"`
	PJSK        PJSKConfig        `yaml:"pjsk"`
	Sekai       SekaiConfig       `yaml:"sekai"`
	PJSKRender  PJSKRenderConfig  `yaml:"pjsk_render"`
	Censor      CensorConfig      `yaml:"censor"`
	HarukiBotDB HarukiBotDBConfig `yaml:"haruki_bot"`
	UsersDB     UsersDBConfig     `yaml:"users_db"`
	Redis       RedisConfig       `yaml:"redis"`
	Toolbox     ToolboxConfig     `yaml:"toolbox"`
	SekaiAPI    SekaiAPIConfig    `yaml:"sekai_api"`
	Tracker     TrackerConfig     `yaml:"tracker"`
}

var Cfg Config

// ApplyProfileDefaults fills in zero-value fields with profile-aware defaults.
// Called after YAML parse but before env overrides, so explicit YAML values and
// env vars always take precedence.
func ApplyProfileDefaults(cfg *Config) {
	p := cfg.Profile

	// LogLevel — only set when the YAML field is empty.
	if cfg.Backend.LogLevel == "" {
		switch {
		case p.IsProduction():
			cfg.Backend.LogLevel = LogLevelWarn
		case p.IsBeta():
			cfg.Backend.LogLevel = LogLevelInfo
		default:
			cfg.Backend.LogLevel = LogLevelDebug
		}
	}

	// APICacheTTL — only set when unspecified (zero).
	if cfg.Backend.APICacheTTL == 0 {
		switch {
		case p.IsProduction():
			cfg.Backend.APICacheTTL = 120 * time.Second
		case p.IsBeta():
			cfg.Backend.APICacheTTL = 60 * time.Second
		default:
			cfg.Backend.APICacheTTL = 10 * time.Second
		}
	}

	// Production safety: force-disable insecure internal API regardless of YAML.
	if p.IsProduction() {
		cfg.Backend.AllowInsecureInternalAPI = false
	}
}

func ReadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return Config{}, fmt.Errorf("failed to unmarshal config file: %w", err)
	}

	// Normalise profile from YAML (may be empty → defaults to "dev").
	p, pErr := ParseProfile(string(cfg.Profile))
	if pErr != nil {
		return Config{}, fmt.Errorf("invalid profile in config: %w", pErr)
	}
	cfg.Profile = p

	ApplyProfileDefaults(&cfg)
	ApplyEnvOverrides(&cfg)
	return cfg, nil
}

func LoadConfig(path string) {
	cfg, err := ReadConfig(path)
	if err != nil {
		log.Fatalf("%v", err)
	}
	Cfg = cfg
}
