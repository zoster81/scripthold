package httptransport

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	pathpkg "path"
	"strconv"
	"strings"
	"time"
)

const (
	EnvAddress               = "MCP_HTTP_ADDR"
	EnvPath                  = "MCP_HTTP_PATH"
	EnvToken                 = "MCP_HTTP_TOKEN"
	EnvTokenFile             = "MCP_HTTP_TOKEN_FILE"
	EnvAllowedHosts          = "MCP_HTTP_ALLOWED_HOSTS"
	EnvAllowedOrigins        = "MCP_HTTP_ALLOWED_ORIGINS"
	EnvAllowNonLoopback      = "MCP_HTTP_ALLOW_NON_LOOPBACK"
	EnvTLSCertFile           = "MCP_HTTP_TLS_CERT_FILE"
	EnvTLSKeyFile            = "MCP_HTTP_TLS_KEY_FILE"
	EnvTrustedProxyCIDRs     = "MCP_HTTP_TRUSTED_PROXY_CIDRS"
	EnvMaxBodyBytes          = "MCP_HTTP_MAX_BODY_BYTES"
	EnvMaxInFlightBodyBytes  = "MCP_HTTP_MAX_INFLIGHT_BODY_BYTES"
	EnvMaxConcurrentRequests = "MCP_HTTP_MAX_CONCURRENT_REQUESTS"
	EnvSessionTimeout        = "MCP_HTTP_SESSION_TIMEOUT"
	EnvEnableExecution       = "MCP_HTTP_ENABLE_EXECUTION"

	DefaultAddress               = "127.0.0.1:8765"
	DefaultPath                  = "/mcp"
	DefaultMaxBodyBytes          = int64(16 * 1024 * 1024)
	DefaultMaxInFlightBodyBytes  = int64(64 * 1024 * 1024)
	DefaultMaxConcurrentRequests = 64
	DefaultSessionTimeout        = 15 * time.Minute

	maxTokenBytes            = 4096
	maxHTTPBodyBytes         = int64(256 * 1024 * 1024)
	maxHTTPInFlightBodyBytes = int64(1024 * 1024 * 1024)
	maxConcurrent            = 4096
	minSessionTimeout        = time.Minute
	maxSessionTimeout        = 24 * time.Hour
	defaultRatePerSec        = 20.0
	defaultRateBurst         = 40.0
	defaultRatePeers         = 4096
	defaultRatePeerIdle      = 10 * time.Minute
)

// Config is the validated native Streamable HTTP configuration.
type Config struct {
	Address               string
	Path                  string
	TokenDigest           [32]byte
	PrincipalID           string
	AllowedHosts          map[string]struct{}
	AllowedOrigins        map[string]struct{}
	AllowNonLoopback      bool
	TLSCertFile           string
	TLSKeyFile            string
	TrustedProxyCIDRs     []netip.Prefix
	ProxyOnlyPlaintext    bool
	MaxBodyBytes          int64
	MaxInFlightBodyBytes  int64
	MaxConcurrentRequests int
	MaxSessions           int
	SessionTimeout        time.Duration
	EnableExecution       bool
}

// ClearCredentialEnvironment removes HTTP credential locations after startup
// configuration has been snapshotted, preventing child-process inheritance.
func ClearCredentialEnvironment() {
	_ = os.Unsetenv(EnvToken)
	_ = os.Unsetenv(EnvTokenFile)
}

// LoadConfig loads and validates HTTP configuration without starting a listener.
func LoadConfig(getenv func(string) string, maxSessions int) (Config, error) {
	if getenv == nil {
		return Config{}, fmt.Errorf("environment reader is required")
	}
	if maxSessions <= 0 {
		return Config{}, fmt.Errorf("maximum sessions must be positive")
	}

	address := strings.TrimSpace(getenv(EnvAddress))
	if address == "" {
		address = DefaultAddress
	}
	host, port, loopback, err := validateAddress(address)
	if err != nil {
		return Config{}, fmt.Errorf("invalid %s: %w", EnvAddress, err)
	}

	endpointPath := strings.TrimSpace(getenv(EnvPath))
	if endpointPath == "" {
		endpointPath = DefaultPath
	}
	if err := validateEndpointPath(endpointPath); err != nil {
		return Config{}, fmt.Errorf("invalid %s: %w", EnvPath, err)
	}

	token, err := loadToken(getenv)
	if err != nil {
		return Config{}, err
	}
	digest := sha256.Sum256(token)
	clear(token)

	allowNonLoopback, err := parseBoolean(getenv(EnvAllowNonLoopback))
	if err != nil {
		return Config{}, fmt.Errorf("invalid %s: %w", EnvAllowNonLoopback, err)
	}
	if !loopback && !allowNonLoopback {
		return Config{}, fmt.Errorf("non-loopback %s requires %s=1", address, EnvAllowNonLoopback)
	}

	certFile := strings.TrimSpace(getenv(EnvTLSCertFile))
	keyFile := strings.TrimSpace(getenv(EnvTLSKeyFile))
	if (certFile == "") != (keyFile == "") {
		return Config{}, fmt.Errorf("%s and %s must be configured together", EnvTLSCertFile, EnvTLSKeyFile)
	}

	trustedProxies, err := parsePrefixes(getenv(EnvTrustedProxyCIDRs))
	if err != nil {
		return Config{}, fmt.Errorf("invalid %s: %w", EnvTrustedProxyCIDRs, err)
	}
	proxyOnlyPlaintext := !loopback && certFile == ""
	if proxyOnlyPlaintext && len(trustedProxies) == 0 {
		return Config{}, fmt.Errorf("non-loopback plaintext requires %s", EnvTrustedProxyCIDRs)
	}

	allowedHosts, err := parseAllowedHosts(getenv(EnvAllowedHosts), host, port, loopback)
	if err != nil {
		return Config{}, fmt.Errorf("invalid %s: %w", EnvAllowedHosts, err)
	}
	allowedOrigins, err := parseAllowedOrigins(getenv(EnvAllowedOrigins))
	if err != nil {
		return Config{}, fmt.Errorf("invalid %s: %w", EnvAllowedOrigins, err)
	}

	maxBodyBytes, err := parsePositiveInt64(getenv(EnvMaxBodyBytes), DefaultMaxBodyBytes, maxHTTPBodyBytes)
	if err != nil {
		return Config{}, fmt.Errorf("invalid %s: %w", EnvMaxBodyBytes, err)
	}
	maxInFlightBodyBytes, err := parsePositiveInt64(
		getenv(EnvMaxInFlightBodyBytes),
		DefaultMaxInFlightBodyBytes,
		maxHTTPInFlightBodyBytes,
	)
	if err != nil {
		return Config{}, fmt.Errorf("invalid %s: %w", EnvMaxInFlightBodyBytes, err)
	}
	if maxInFlightBodyBytes < maxBodyBytes {
		return Config{}, fmt.Errorf("%s must be at least %s", EnvMaxInFlightBodyBytes, EnvMaxBodyBytes)
	}
	maxRequests, err := parsePositiveInt(getenv(EnvMaxConcurrentRequests), DefaultMaxConcurrentRequests, maxConcurrent)
	if err != nil {
		return Config{}, fmt.Errorf("invalid %s: %w", EnvMaxConcurrentRequests, err)
	}
	sessionTimeout, err := parseDuration(getenv(EnvSessionTimeout), DefaultSessionTimeout, minSessionTimeout, maxSessionTimeout)
	if err != nil {
		return Config{}, fmt.Errorf("invalid %s: %w", EnvSessionTimeout, err)
	}
	enableExecution, err := parseBoolean(getenv(EnvEnableExecution))
	if err != nil {
		return Config{}, fmt.Errorf("invalid %s: %w", EnvEnableExecution, err)
	}

	return Config{
		Address:               address,
		Path:                  endpointPath,
		TokenDigest:           digest,
		PrincipalID:           "static:" + hex.EncodeToString(digest[:]),
		AllowedHosts:          allowedHosts,
		AllowedOrigins:        allowedOrigins,
		AllowNonLoopback:      allowNonLoopback,
		TLSCertFile:           certFile,
		TLSKeyFile:            keyFile,
		TrustedProxyCIDRs:     trustedProxies,
		ProxyOnlyPlaintext:    proxyOnlyPlaintext,
		MaxBodyBytes:          maxBodyBytes,
		MaxInFlightBodyBytes:  maxInFlightBodyBytes,
		MaxConcurrentRequests: maxRequests,
		MaxSessions:           maxSessions,
		SessionTimeout:        sessionTimeout,
		EnableExecution:       enableExecution,
	}, nil
}

func (config Config) UseTLS() bool {
	return config.TLSCertFile != "" && config.TLSKeyFile != ""
}

func (config Config) AllowsHost(host string) bool {
	normalized, err := normalizeHostValue(host)
	if err != nil {
		return false
	}
	_, ok := config.AllowedHosts[normalized]
	return ok
}

func (config Config) AllowsOrigin(origin string) bool {
	normalized, err := normalizeOrigin(origin)
	if err != nil {
		return false
	}
	_, ok := config.AllowedOrigins[normalized]
	return ok
}

func validateAddress(address string) (host, port string, loopback bool, err error) {
	host, port, err = net.SplitHostPort(address)
	if err != nil {
		return "", "", false, err
	}
	if strings.TrimSpace(host) == "" {
		return "", "", false, fmt.Errorf("host is required")
	}
	parsedPort, err := strconv.Atoi(port)
	if err != nil || parsedPort < 1 || parsedPort > 65535 {
		return "", "", false, fmt.Errorf("port must be in 1..65535")
	}

	if strings.EqualFold(host, "localhost") {
		return host, port, true, nil
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return "", "", false, fmt.Errorf("host must be localhost or an IP literal")
	}
	return host, port, ip.IsLoopback(), nil
}

func validateEndpointPath(endpointPath string) error {
	if !strings.HasPrefix(endpointPath, "/") {
		return fmt.Errorf("path must be absolute")
	}
	parsed, err := url.ParseRequestURI(endpointPath)
	if err != nil {
		return err
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("query and fragment are forbidden")
	}
	if cleaned := pathpkg.Clean(endpointPath); cleaned != endpointPath {
		return fmt.Errorf("path must already be clean")
	}
	if endpointPath == "/healthz" || endpointPath == "/readyz" {
		return fmt.Errorf("path conflicts with a health endpoint")
	}
	return nil
}

func loadToken(getenv func(string) string) ([]byte, error) {
	environmentToken := getenv(EnvToken)
	filePath := strings.TrimSpace(getenv(EnvTokenFile))
	if environmentToken == "" && filePath == "" {
		return nil, fmt.Errorf("exactly one of %s or %s is required", EnvToken, EnvTokenFile)
	}
	if environmentToken != "" && filePath != "" {
		return nil, fmt.Errorf("%s and %s are mutually exclusive", EnvToken, EnvTokenFile)
	}

	var token []byte
	if filePath != "" {
		info, err := os.Stat(filePath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", EnvTokenFile, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s must reference a regular file", EnvTokenFile)
		}
		if info.Size() > maxTokenBytes+2 {
			return nil, fmt.Errorf("%s exceeds %d bytes", EnvTokenFile, maxTokenBytes)
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", EnvTokenFile, err)
		}
		token = trimOneLineEnding(data)
	} else {
		token = []byte(environmentToken)
	}

	if len(token) < 32 {
		return nil, fmt.Errorf("bearer token must contain at least 32 visible ASCII bytes")
	}
	if len(token) > maxTokenBytes {
		return nil, fmt.Errorf("bearer token exceeds %d bytes", maxTokenBytes)
	}
	for _, value := range token {
		if value < 0x21 || value > 0x7e {
			return nil, fmt.Errorf("bearer token must contain visible ASCII without whitespace")
		}
	}
	return token, nil
}

func trimOneLineEnding(data []byte) []byte {
	if len(data) >= 2 && data[len(data)-2] == '\r' && data[len(data)-1] == '\n' {
		return data[:len(data)-2]
	}
	if len(data) >= 1 && data[len(data)-1] == '\n' {
		return data[:len(data)-1]
	}
	return data
}

func parseAllowedHosts(value, listenerHost, port string, loopback bool) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	add := func(host string) error {
		normalized, err := normalizeHostValue(host)
		if err != nil {
			return err
		}
		result[normalized] = struct{}{}
		return nil
	}
	if err := add(net.JoinHostPort(listenerHost, port)); err != nil {
		return nil, err
	}
	if loopback {
		for _, host := range []string{
			net.JoinHostPort("localhost", port),
			net.JoinHostPort("127.0.0.1", port),
			net.JoinHostPort("::1", port),
		} {
			if err := add(host); err != nil {
				return nil, err
			}
		}
	}
	for _, host := range splitCommaList(value) {
		if err := add(host); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func parseAllowedOrigins(value string) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	for _, origin := range splitCommaList(value) {
		normalized, err := normalizeOrigin(origin)
		if err != nil {
			return nil, err
		}
		result[normalized] = struct{}{}
	}
	return result, nil
}

func normalizeOrigin(origin string) (string, error) {
	origin = strings.TrimSpace(origin)
	if origin == "" || origin == "null" || strings.Contains(origin, "*") {
		return "", fmt.Errorf("origin %q is not an exact HTTP origin", origin)
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return "", err
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("origin scheme must be http or https")
	}
	if parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("origin %q must contain only scheme and host", origin)
	}
	host, err := normalizeHostValue(parsed.Host)
	if err != nil {
		return "", err
	}
	if scheme == "http" && strings.HasSuffix(host, ":80") {
		host = strings.TrimSuffix(host, ":80")
	}
	if scheme == "https" && strings.HasSuffix(host, ":443") {
		host = strings.TrimSuffix(host, ":443")
	}
	return scheme + "://" + host, nil
}

func normalizeHostValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "*") || strings.Contains(value, "://") {
		return "", fmt.Errorf("host %q is not an exact Host value", value)
	}
	for _, current := range value {
		if current <= 0x20 || current >= 0x7f || current == '\\' {
			return "", fmt.Errorf("host %q contains unsupported characters", value)
		}
	}
	parsed, err := url.Parse("http://" + value)
	if err != nil {
		return "", fmt.Errorf("host %q is invalid: %w", value, err)
	}
	if parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Host != value {
		return "", fmt.Errorf("host %q is not an exact Host value", value)
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return "", fmt.Errorf("host %q has no hostname", value)
	}
	port := parsed.Port()
	if strings.HasSuffix(value, ":") {
		return "", fmt.Errorf("host %q has an empty port", value)
	}
	if port != "" {
		parsedPort, err := strconv.Atoi(port)
		if err != nil || parsedPort < 1 || parsedPort > 65535 {
			return "", fmt.Errorf("host %q has an invalid port", value)
		}
	}

	if address, err := netip.ParseAddr(hostname); err == nil {
		hostname = address.Unmap().String()
		if port != "" {
			return strings.ToLower(net.JoinHostPort(hostname, port)), nil
		}
		if strings.Contains(hostname, ":") {
			return "[" + strings.ToLower(hostname) + "]", nil
		}
		return strings.ToLower(hostname), nil
	}
	if err := validateDNSName(hostname); err != nil {
		return "", fmt.Errorf("host %q is invalid: %w", value, err)
	}
	if port != "" {
		return net.JoinHostPort(hostname, port), nil
	}
	return hostname, nil
}

func validateDNSName(hostname string) error {
	if len(hostname) > 253 || strings.HasPrefix(hostname, ".") || strings.HasSuffix(hostname, ".") {
		return fmt.Errorf("invalid DNS name")
	}
	for _, label := range strings.Split(hostname, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("invalid DNS label")
		}
		for _, current := range label {
			if (current < 'a' || current > 'z') && (current < '0' || current > '9') && current != '-' {
				return fmt.Errorf("invalid DNS character")
			}
		}
	}
	return nil
}

func parsePrefixes(value string) ([]netip.Prefix, error) {
	var result []netip.Prefix
	for _, item := range splitCommaList(value) {
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			return nil, err
		}
		result = append(result, prefix.Masked())
	}
	return result, nil
}

func splitCommaList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parseBoolean(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return false, nil
	case "1", "true", "yes", "on", "enabled":
		return true, nil
	case "0", "false", "no", "off", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("expected a boolean value")
	}
}

func parsePositiveInt64(value string, fallback, maximum int64) (int64, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 || parsed > maximum {
		return 0, fmt.Errorf("expected an integer in 1..%d", maximum)
	}
	return parsed, nil
}

func parsePositiveInt(value string, fallback, maximum int) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 || parsed > maximum {
		return 0, fmt.Errorf("expected an integer in 1..%d", maximum)
	}
	return parsed, nil
}

func parseDuration(value string, fallback, minimum, maximum time.Duration) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("expected a duration in %s..%s", minimum, maximum)
	}
	return parsed, nil
}
