package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

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

// ApplyEnvOverrides replaces key config fields with environment variables when set.
// Env var names follow the pattern HARUKI_<SECTION>_<FIELD> (all upper-snake).
func ApplyEnvOverrides(cfg *Config) {
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
	envStr("HARUKI_BOT_TURNSTILE_SECRET", &cfg.HarukiBotDB.TurnstileSecretKey)
	envStr("HARUKI_BOT_SMTP_HOST", &cfg.HarukiBotDB.SMTPHost)
	envInt("HARUKI_BOT_SMTP_PORT", &cfg.HarukiBotDB.SMTPPort)
	envStr("HARUKI_BOT_SMTP_USERNAME", &cfg.HarukiBotDB.SMTPUsername)
	envStr("HARUKI_BOT_SMTP_PASSWORD", &cfg.HarukiBotDB.SMTPPassword)
	envStr("HARUKI_BOT_SMTP_FROM", &cfg.HarukiBotDB.SMTPFrom)
	envStr("HARUKI_BOT_CREDENTIAL_SIGN_TOKEN", &cfg.HarukiBotDB.CredentialSignToken)
	envStr("HARUKI_BOT_SESSION_SIGN_TOKEN", &cfg.HarukiBotDB.SessionSignToken)
	envStr("HARUKI_BOT_INTERNAL_API_TOKEN", &cfg.HarukiBotDB.InternalAPIToken)
	envInt("HARUKI_BOT_SESSION_TTL_DAYS", &cfg.HarukiBotDB.SessionTTLDays)
	envStr("HARUKI_BOT_NOISE_PRIVATE_KEY", &cfg.HarukiBotDB.NoisePrivateKey)

	// Sekai API
	envStr("HARUKI_SEKAI_API_BASE_URL", &cfg.SekaiAPI.BaseURL)
	envStr("HARUKI_SEKAI_API_TOKEN", &cfg.SekaiAPI.Token)

	// Toolbox
	envStr("HARUKI_TOOLBOX_BASE_URL", &cfg.Toolbox.BaseURL)
	envStr("HARUKI_TOOLBOX_API_TOKEN", &cfg.Toolbox.APIToken)

	// Tracker
	envStr("HARUKI_TRACKER_BASE_URL", &cfg.Tracker.BaseURL)

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
	envStr("HARUKI_PJSK_RENDER_IMAGE_CACHE_PG_URL", &cfg.PJSKRender.ImageCache.PGURL)
	envStr("HARUKI_PJSK_RENDER_ASSETS_BASE_URL", &cfg.PJSKRender.AssetDirs.AssetsBaseURL)
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
}

type ChunithmConfig struct {
	Enabled       bool   `yaml:"enabled"`
	MusicDBType   string `yaml:"music_db_type"`
	MusicDBURL    string `yaml:"music_db_url"`
	BindingDBType string `yaml:"binding_db_type"`
	BindingDBURL  string `yaml:"binding_db_url"`
}

type PJSKParserConfig struct {
	ChardataRegion          string        `yaml:"chardata_region"`
	ChardataRefreshInterval time.Duration `yaml:"chardata_refresh_interval"`
}

type PJSKConfig struct {
	Enabled        bool                      `yaml:"enabled"`
	DBType         string                    `yaml:"db_type"`
	DBURL          string                    `yaml:"db_url"`
	Parser         PJSKParserConfig          `yaml:"parser"`
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
	Enabled bool   `yaml:"enabled"`
	Dir     string `yaml:"dir"`
}

type UserSnapshotConfig struct {
	Provider      string `yaml:"provider"`
	UserJSON      string `yaml:"user_json"`
	MusicMetaJSON string `yaml:"music_meta_json"`
	MySekaiJSON   string `yaml:"mysekai_json"`
}

type DeckRecommendConfig struct {
	Enabled          bool          `yaml:"enabled"`
	UseLocalEngine   bool          `yaml:"use_local_engine"`
	ServiceBaseURL   string        `yaml:"service_base_url"`
	LocalPoolSize    int           `yaml:"local_pool_size"`
	LocalLibraryDirs []string      `yaml:"local_library_dirs"`
	StaticDataDir    string        `yaml:"static_data_dir"`
	Timeout          time.Duration `yaml:"timeout"`
	DefaultAlgs      []string      `yaml:"default_algs"`
}

type RenderCacheConfig struct {
	BaseURL    string        `yaml:"base_url"`
	StorageDir string        `yaml:"storage_dir"`
	TTL        time.Duration `yaml:"ttl"`
}

type ImageCacheConfig struct {
	URI   string `yaml:"uri"`
	Dir   string `yaml:"dir"`
	PGURL string `yaml:"pg_url"` // PostgreSQL DSN for deduplication store (optional)
}

type MusicMetaConfig struct {
	RefreshInterval time.Duration `yaml:"refresh_interval"` // default: 30m
}

// MySekaiCNWhitelistEntry defines a platform+group pair allowed to use
// MySekai features on the CN region.
type MySekaiCNWhitelistEntry struct {
	Platform string `yaml:"platform"`
	GroupID  string `yaml:"group_id"`
}

type PJSKRenderConfig struct {
	Enabled           bool                  `yaml:"enabled"`
	DrawingBaseURL    string                `yaml:"drawing_base_url"`
	DrawingTimeout    time.Duration         `yaml:"drawing_timeout"`
	DrawingRetryCount int                   `yaml:"drawing_retry_count"`
	DrawingCache      RenderCacheConfig     `yaml:"drawing_cache"`
	ImageCache        ImageCacheConfig      `yaml:"image_cache"`
	AssetDirs         AssetDirsConfig       `yaml:"asset_dirs"`
	LocalMasterdata   LocalMasterdataConfig `yaml:"local_masterdata"`
	UserSnapshot      UserSnapshotConfig    `yaml:"user_snapshot"`
	MusicMeta         MusicMetaConfig       `yaml:"music_meta"`
	DeckRecommend     DeckRecommendConfig   `yaml:"deck_recommend"`
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
	TurnstileSecretKey  string `yaml:"turnstile_secret_key"`
	SMTPHost            string `yaml:"smtp_host"`
	SMTPPort            int    `yaml:"smtp_port"`
	SMTPUsername        string `yaml:"smtp_username"`
	SMTPPassword        string `yaml:"smtp_password"`
	SMTPFrom            string `yaml:"smtp_from"`
	CredentialSignToken string `yaml:"credential_sign_token"`
	SessionSignToken    string `yaml:"session_sign_token"`
	InternalAPIToken    string `yaml:"internal_api_token"`
	SessionTTLDays      int    `yaml:"session_ttl_days"`
	NoisePrivateKey     string `yaml:"noise_private_key"`
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
	BaseURL string `yaml:"base_url"`
	Token   string `yaml:"token"`
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

func LoadConfig(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("failed to read config file: %v", err)
	}

	err = yaml.Unmarshal(data, &Cfg)
	if err != nil {
		log.Fatalf("failed to unmarshal config file: %v", err)
	}

	ApplyEnvOverrides(&Cfg)
}
