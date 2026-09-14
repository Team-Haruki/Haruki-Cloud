package storage

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Canonical scheme values accepted by Resolve.
const (
	SchemeFS = "fs"
	SchemeS3 = "s3"

	schemeLocalAlias = "local"
	defaultS3Region  = "garage"
)

// Option keys derived by Resolve (opendal names) and the typed s3 options.
const (
	OptionRoot                   = "root"
	OptionBucket                 = "bucket"
	OptionEndpoint               = "endpoint"
	OptionRegion                 = "region"
	OptionAccessKeyID            = "access_key_id"
	OptionSecretAccessKey        = "secret_access_key"
	OptionDefaultACL             = "default_acl"
	OptionEnableVirtualHostStyle = "enable_virtual_host_style"

	OptionRequestTimeout   = "request_timeout"
	OptionDialTimeout      = "dial_timeout"
	OptionStatTimeout      = "stat_timeout"
	OptionFailoverCooldown = "failover_cooldown"
	OptionMaxAttempts      = "max_attempts"
	OptionMaxObjectBytes   = "max_object_bytes"
	OptionProxy            = "proxy"

	aclPublicRead = "public-read"
)

var knownOptionKeys = []string{
	OptionRoot, OptionBucket, OptionEndpoint, OptionRegion, OptionAccessKeyID, OptionSecretAccessKey,
	OptionDefaultACL, OptionEnableVirtualHostStyle, OptionRequestTimeout, OptionDialTimeout,
	OptionStatTimeout, OptionFailoverCooldown, OptionMaxAttempts, OptionMaxObjectBytes, OptionProxy,
}

// ProviderConfig is one storage slot's provider block. Key names follow the
// Asset-Updater StorageProviderConfig schema; endpoints is the only Cloud
// addition. The yaml aliases name, kind and public_base_url fill Provider,
// Scheme and BaseURL when the canonical key is absent.
type ProviderConfig struct {
	Provider        string            `yaml:"provider"`
	Scheme          string            `yaml:"scheme"`
	Endpoint        string            `yaml:"endpoint"`
	Endpoints       []string          `yaml:"endpoints"`
	TLS             *bool             `yaml:"tls"`
	Bucket          string            `yaml:"bucket"`
	Root            string            `yaml:"root"`
	Prefix          string            `yaml:"prefix"`
	Region          string            `yaml:"region"`
	AccessKeyID     string            `yaml:"access_key_id"`
	SecretAccessKey string            `yaml:"secret_access_key"`
	PublicRead      bool              `yaml:"public_read"`
	PathStyle       *bool             `yaml:"path_style"`
	BaseURL         string            `yaml:"base_url"`
	Options         map[string]string `yaml:"options"`
	Mirror          *ProviderConfig   `yaml:"mirror"`
	MirrorMode      string            `yaml:"mirror_mode"`
}

// rawProviderConfig has ProviderConfig's fields without its methods, so the
// alias decoder can inline it without recursing into UnmarshalYAML.
type rawProviderConfig ProviderConfig

type providerConfigYAML struct {
	rawProviderConfig `yaml:",inline"`
	Name              string `yaml:"name"`
	Kind              string `yaml:"kind"`
	PublicBaseURL     string `yaml:"public_base_url"`
}

// UnmarshalYAML decodes a provider block, applying the name, kind and
// public_base_url aliases.
func (c *ProviderConfig) UnmarshalYAML(value *yaml.Node) error {
	var shadow providerConfigYAML
	if err := value.Decode(&shadow); err != nil {
		return err
	}
	decoded := ProviderConfig(shadow.rawProviderConfig)
	if strings.TrimSpace(decoded.Provider) == "" {
		decoded.Provider = shadow.Name
	}
	if strings.TrimSpace(decoded.Scheme) == "" {
		decoded.Scheme = shadow.Kind
	}
	if strings.TrimSpace(decoded.BaseURL) == "" {
		decoded.BaseURL = shadow.PublicBaseURL
	}
	*c = decoded
	return nil
}

// IsZero reports whether the block configures nothing, so the slot falls back
// to its legacy root.
func (c ProviderConfig) IsZero() bool {
	return c.Provider == "" && c.Scheme == "" && c.Endpoint == "" && len(c.Endpoints) == 0 &&
		c.TLS == nil && c.Bucket == "" && c.Root == "" && c.Prefix == "" && c.Region == "" &&
		c.AccessKeyID == "" && c.SecretAccessKey == "" && !c.PublicRead && c.PathStyle == nil &&
		c.BaseURL == "" && len(c.Options) == 0 && c.Mirror == nil && c.MirrorMode == ""
}

// Resolved mirrors Asset-Updater's ResolvedStorageProvider plus the endpoint
// list. Options is an opendal-shaped option map; it carries the credentials,
// so a Resolved value must never be logged whole.
type Resolved struct {
	Provider   string
	Scheme     string
	Endpoints  []string
	Bucket     string
	Root       string
	BaseURL    string
	PublicRead bool
	PathStyle  bool
	Region     string
	Options    map[string]string
	// Warnings lists non-fatal findings (ignored endpoint, unknown options)
	// for the caller to log.
	Warnings []string
}

// HasCredentials reports whether both S3 credentials are present.
func (r Resolved) HasCredentials() bool {
	return r.Options[OptionAccessKeyID] != "" && r.Options[OptionSecretAccessKey] != ""
}

// Resolve is a port of Asset-Updater's resolve_storage_provider with Cloud's
// deviations: the default scheme is fs, "local" is an alias of fs, and an
// endpoints list wins over endpoint.
func Resolve(c ProviderConfig) (Resolved, error) {
	scheme, err := normalizeScheme(c.Scheme)
	if err != nil {
		return Resolved{}, err
	}
	resolved := Resolved{Scheme: scheme, Provider: strings.TrimSpace(c.Provider)}
	if resolved.Provider == "" {
		resolved.Provider = scheme
	}
	options := make(map[string]string, len(c.Options)+8)
	for key, value := range c.Options {
		options[key] = value
	}
	resolved.Endpoints, resolved.Warnings = resolveEndpoints(c)
	root := resolveRoot(c, scheme)
	orInsert(options, OptionRoot, root)
	orInsert(options, OptionBucket, strings.TrimSpace(c.Bucket))
	if len(resolved.Endpoints) > 0 {
		orInsert(options, OptionEndpoint, resolved.Endpoints[0])
	}
	region := strings.TrimSpace(c.Region)
	if region == "" && scheme == SchemeS3 {
		region = defaultS3Region
	}
	orInsert(options, OptionRegion, region)
	orInsert(options, OptionAccessKeyID, c.AccessKeyID)
	orInsert(options, OptionSecretAccessKey, c.SecretAccessKey)
	if scheme == SchemeS3 && c.PublicRead {
		orInsert(options, OptionDefaultACL, aclPublicRead)
	}
	if c.PathStyle != nil && !*c.PathStyle {
		orInsert(options, OptionEnableVirtualHostStyle, "true")
	}

	resolved.Options = options
	resolved.Bucket = strings.TrimSpace(options[OptionBucket])
	resolved.Root = normalizeResolvedRoot(options[OptionRoot], scheme)
	resolved.Region = options[OptionRegion]
	resolved.PublicRead = strings.EqualFold(options[OptionDefaultACL], aclPublicRead)
	resolved.PathStyle = !strings.EqualFold(strings.TrimSpace(options[OptionEnableVirtualHostStyle]), "true")
	if scheme == SchemeS3 {
		resolved.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
		if resolved.BaseURL == "" && len(resolved.Endpoints) > 0 {
			resolved.BaseURL = resolved.Endpoints[0]
		}
	} else {
		resolved.Endpoints = nil
	}
	if unknown := unknownOptionKeys(c.Options); len(unknown) > 0 {
		resolved.Warnings = append(resolved.Warnings, "unknown options kept: "+strings.Join(unknown, ","))
	}
	return resolved, nil
}

func normalizeScheme(raw string) (string, error) {
	scheme := strings.ToLower(strings.TrimSpace(raw))
	switch scheme {
	case "", schemeLocalAlias, SchemeFS:
		return SchemeFS, nil
	case SchemeS3:
		return SchemeS3, nil
	default:
		return "", fmt.Errorf("scheme: unsupported scheme %q (want %q or %q)", raw, SchemeFS, SchemeS3)
	}
}

func orInsert(options map[string]string, key, value string) {
	if value == "" {
		return
	}
	if _, ok := options[key]; !ok {
		options[key] = value
	}
}

func resolveRoot(c ProviderConfig, scheme string) string {
	if root := strings.TrimSpace(c.Root); root != "" {
		return root
	}
	prefix := strings.TrimSpace(c.Prefix)
	if scheme == SchemeS3 {
		prefix = strings.Trim(strings.ReplaceAll(prefix, "\\", "/"), "/")
	}
	return prefix
}

func normalizeResolvedRoot(root, scheme string) string {
	root = strings.TrimSpace(root)
	if scheme == SchemeS3 {
		return strings.Trim(strings.ReplaceAll(root, "\\", "/"), "/")
	}
	if root == "" {
		return ""
	}
	return filepath.Clean(root)
}

func resolveEndpoints(c ProviderConfig) ([]string, []string) {
	var warnings []string
	raw := c.Endpoints
	if len(nonEmpty(raw)) == 0 {
		raw = []string{c.Endpoint}
	} else if strings.TrimSpace(c.Endpoint) != "" {
		warnings = append(warnings, fmt.Sprintf("endpoint %q ignored because endpoints is set", strings.TrimSpace(c.Endpoint)))
	}
	endpoints := make([]string, 0, len(raw))
	for _, value := range raw {
		built := constructProviderEndpoint(value, c.TLS)
		if built == "" || slices.Contains(endpoints, built) {
			continue
		}
		endpoints = append(endpoints, built)
	}
	return endpoints, warnings
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

// constructProviderEndpoint ports Asset-Updater's construct_provider_endpoint:
// full URLs are kept (trailing slash trimmed), bare host[:port] values get
// https unless tls is explicitly false.
func constructProviderEndpoint(raw string, tls *bool) string {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return value
	}
	if tls != nil && !*tls {
		return "http://" + value
	}
	return "https://" + value
}

func unknownOptionKeys(options map[string]string) []string {
	var unknown []string
	for key := range options {
		if !slices.Contains(knownOptionKeys, key) {
			unknown = append(unknown, key)
		}
	}
	slices.Sort(unknown)
	return unknown
}

// Validate reports the first configuration error of a slot's provider block.
// Every error names the slot and the offending key.
func Validate(slot string, c ProviderConfig) error {
	if err := validateProvider(c); err != nil {
		return fmt.Errorf("storage.%s.%w", slot, err)
	}
	if c.Mirror == nil {
		if strings.TrimSpace(c.MirrorMode) != "" {
			return fmt.Errorf("storage.%s.mirror_mode: set without mirror", slot)
		}
		return nil
	}
	if Slot(slot) != SlotUserUpload {
		return fmt.Errorf("storage.%s.mirror: only the %s slot may configure a mirror", slot, SlotUserUpload)
	}
	switch DualMode(strings.TrimSpace(c.MirrorMode)) {
	case "", DualWrite, DualWriteOnly:
	default:
		return fmt.Errorf("storage.%s.mirror_mode: unsupported value %q (want %q or %q)", slot, c.MirrorMode, DualWrite, DualWriteOnly)
	}
	if c.Mirror.Mirror != nil || strings.TrimSpace(c.Mirror.MirrorMode) != "" {
		return fmt.Errorf("storage.%s.mirror.mirror: a mirror may not carry a mirror", slot)
	}
	if err := validateProvider(*c.Mirror); err != nil {
		return fmt.Errorf("storage.%s.mirror.%w", slot, err)
	}
	return nil
}

func validateProvider(c ProviderConfig) error {
	resolved, err := Resolve(c)
	if err != nil {
		return err
	}
	if (resolved.Options[OptionAccessKeyID] == "") != (resolved.Options[OptionSecretAccessKey] == "") {
		return errors.New("access_key_id: access_key_id and secret_access_key must be set together")
	}
	if resolved.Scheme == SchemeFS {
		if resolved.Root == "" {
			return errors.New("root: required for scheme fs")
		}
		if !filepath.IsAbs(resolved.Root) {
			return fmt.Errorf("root: %q must be an absolute path for scheme fs", resolved.Root)
		}
		return nil
	}
	if resolved.Bucket == "" {
		return errors.New("bucket: required for scheme s3")
	}
	if len(resolved.Endpoints) == 0 {
		return errors.New("endpoints: at least one endpoint is required for scheme s3")
	}
	for _, endpoint := range resolved.Endpoints {
		parsed, parseErr := url.Parse(endpoint)
		if parseErr != nil || parsed.Host == "" {
			return fmt.Errorf("endpoints: %q is not a URL with a host", endpoint)
		}
	}
	return nil
}
