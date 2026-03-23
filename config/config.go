package config

import (
	"log"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type BackendConfig struct {
	Host                string        `yaml:"host"`
	Port                int           `yaml:"port"`
	SSL                 bool          `yaml:"ssl"`
	SSLCert             string        `yaml:"ssl_cert"`
	SSLKey              string        `yaml:"ssl_key"`
	LogLevel            string        `yaml:"log_level"`
	MainLogFile         string        `yaml:"main_log_file"`
	AccessLog           string        `yaml:"access_log"`
	APICacheTTL         time.Duration `yaml:"api_cache_ttl"`
	AccessLogPath       string        `yaml:"access_log_path"`
	AcceptAuthorization string        `yaml:"accept_authorization"`
	AcceptUserAgent     string        `yaml:"accept_user_agent"`
	EnableTrustProxy    bool          `yaml:"enable_trust_proxy"`
	TrustProxies        []string      `yaml:"trusted_proxies"`
	ProxyHeader         string        `yaml:"proxy_header"`
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
	Enabled bool             `yaml:"enabled"`
	DBType  string           `yaml:"db_type"`
	DBURL   string           `yaml:"db_url"`
	Parser  PJSKParserConfig `yaml:"parser"`
}

type SekaiConfig struct {
	Enabled bool   `yaml:"enabled"`
	DBType  string `yaml:"db_type"`
	DBURL   string `yaml:"db_url"`
}

type AssetDirsConfig struct {
	Primary string   `yaml:"primary"`
	Legacy  []string `yaml:"legacy"`
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
	Enabled        bool          `yaml:"enabled"`
	UseLocalEngine bool          `yaml:"use_local_engine"`
	Timeout        time.Duration `yaml:"timeout"`
	DefaultAlgs    []string      `yaml:"default_algs"`
}

type RenderCacheConfig struct {
	BaseURL    string        `yaml:"base_url"`
	StorageDir string        `yaml:"storage_dir"`
	TTL        time.Duration `yaml:"ttl"`
}

type PJSKRenderConfig struct {
	Enabled           bool                  `yaml:"enabled"`
	DrawingBaseURL    string                `yaml:"drawing_base_url"`
	DrawingTimeout    time.Duration         `yaml:"drawing_timeout"`
	DrawingRetryCount int                   `yaml:"drawing_retry_count"`
	DrawingCache      RenderCacheConfig     `yaml:"drawing_cache"`
	AssetDirs         AssetDirsConfig       `yaml:"asset_dirs"`
	LocalMasterdata   LocalMasterdataConfig `yaml:"local_masterdata"`
	UserSnapshot      UserSnapshotConfig    `yaml:"user_snapshot"`
	DeckRecommend     DeckRecommendConfig   `yaml:"deck_recommend"`
}

type CensorConfig struct {
	BaiduAPIKey  string `yaml:"baidu_api_key"`
	BaiduSecret  string `yaml:"baidu_secret"`
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
}
