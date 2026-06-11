package usererror

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

var urlPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

var sensitiveQueryKeys = map[string]struct{}{
	"access_token":  {},
	"api_key":       {},
	"apikey":        {},
	"auth":          {},
	"authorization": {},
	"bearer":        {},
	"jwt":           {},
	"password":      {},
	"passwd":        {},
	"secret":        {},
	"session":       {},
	"session_id":    {},
	"sid":           {},
	"token":         {},
}

// MessageContainsSensitiveURL reports whether a user-facing message includes a
// URL that could expose internal services, upstream game APIs, or credentials.
func MessageContainsSensitiveURL(message string) bool {
	for _, raw := range urlPattern.FindAllString(message, -1) {
		if IsSensitiveURL(raw) {
			return true
		}
	}
	return false
}

func IsSensitiveURL(raw string) bool {
	raw = trimURLPunctuation(raw)
	parsed, err := url.Parse(raw)
	if err != nil {
		return true
	}
	if parsed.User != nil {
		return true
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" {
		return true
	}
	return isSensitiveHost(host) || hasSensitiveQuery(parsed.RawQuery) || hasSensitivePath(parsed)
}

func trimURLPunctuation(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), ".,;:!?，。；：！？)]}）】》")
}

func isSensitiveHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if !strings.Contains(host, ".") {
		return true
	}
	for _, suffix := range []string{
		".cluster.local",
		".docker",
		".internal",
		".lan",
		".local",
		".ts.net",
	} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return strings.Contains(host, "game-api") && strings.HasSuffix(host, ".colorfulpalette.org")
}

func hasSensitiveQuery(rawQuery string) bool {
	if rawQuery == "" {
		return false
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		lower := strings.ToLower(rawQuery)
		return strings.Contains(lower, "token=") ||
			strings.Contains(lower, "authorization=") ||
			strings.Contains(lower, "secret=")
	}
	for key := range values {
		if _, ok := sensitiveQueryKeys[strings.ToLower(strings.TrimSpace(key))]; ok {
			return true
		}
	}
	return false
}

func hasSensitivePath(parsed *url.URL) bool {
	if parsed == nil {
		return false
	}
	path := strings.ToLower(parsed.EscapedPath() + "\n" + parsed.Path)
	return strings.Contains(path, "/api/private/") ||
		strings.Contains(path, "/private/")
}
