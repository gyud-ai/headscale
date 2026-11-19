package types

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/juanfont/headscale/hscontrol/util"
	"github.com/rs/zerolog"
)

// Parse errors of the value types below are *ConfigError without the key;
// the loader adds the key and where the value came from.

// Secret is a credential as written: a literal, or "file:<path>" naming
// the file that holds it (Tailscale's form), which keeps the secret out of
// the config and the environment. Only fmt's String path redacts; JSON,
// %#v and zerolog's Interface print the raw value.
type Secret string

// Resolve returns the literal, or the trimmed contents of the file. The
// path is expanded so a systemd credential can be named by
// ${CREDENTIALS_DIRECTORY}. Errors name the path; callers add the key.
func (s Secret) Resolve() (string, error) {
	path, ok := strings.CutPrefix(string(s), "file:")
	if !ok {
		return string(s), nil
	}

	path = os.ExpandEnv(path)

	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// An empty file is a credential that was not written, not a choice.
	secret := strings.TrimSpace(string(b))
	if secret == "" {
		return "", fmt.Errorf("%s: %w", path, errSecretEmpty)
	}

	return secret, nil
}

// IsFile reports whether s names a file.
func (s Secret) IsFile() bool { return strings.HasPrefix(string(s), "file:") }

// Redacted returns s safe to show: a file reference as written, a literal
// as <redacted>.
func (s Secret) Redacted() string {
	if s == "" || s.IsFile() {
		return string(s)
	}

	return "<redacted>"
}

func (s Secret) String() string { return s.Redacted() }

// FileMode is a permission written as quoted octal text ("0770", "0o770").
type FileMode fs.FileMode

func (m *FileMode) UnmarshalText(text []byte) error {
	mode, err := util.ParseFileMode(string(text))
	if err != nil {
		return &ConfigError{
			Reason:  "is not a quoted octal file mode",
			Maximum: `"0777"`,
			Hint:    `quote the value, e.g. "0770"; unquoted, YAML reads 0770 as the number 504`,
			Cause:   err,
		}
	}

	*m = FileMode(mode)

	return nil
}

func (m FileMode) MarshalText() ([]byte, error) {
	return fmt.Appendf(nil, "%04o", uint32(m)), nil
}

// HTTPURL is "" or an absolute http:// or https:// URL with a hostname, kept
// as written. JSON shows it without userinfo; it has no MarshalText, so the
// text form stays as written.
type HTTPURL string //nolint:recvcheck // UnmarshalText needs a pointer; the rest read the value

// MarshalJSON writes u [HTTPURL.Redacted], for debug output and logs.
func (u HTTPURL) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.Redacted())
}

// Redacted is u without userinfo: a username can be a token too. One that
// is not an absolute http(s) URL with a host is withheld, as its userinfo
// cannot be told from the rest (https://u:p?@h fails to parse,
// https:/u:p@h parses as a path); one without userinfo is kept as written.
func (u HTTPURL) Redacted() string {
	if u == "" {
		return ""
	}

	p, err := url.Parse(string(u))
	if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Hostname() == "" {
		return "<redacted>"
	}

	if p.User == nil {
		return string(u)
	}

	p.User = nil

	return p.String()
}

func (u *HTTPURL) UnmarshalText(text []byte) error {
	s := string(text)
	if s != "" {
		p, err := url.Parse(s)
		if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Hostname() == "" {
			e := &ConfigError{
				Reason: "is not an http:// or https:// URL",
				Hint:   `write the scheme and host, e.g. "https://example.com"`,
			}
			if err != nil {
				// Not the parser's error: it quotes the URL, userinfo included.
				e.Cause = errURLSyntax
			}

			return e
		}
	}

	*u = HTTPURL(s)

	return nil
}

// WebURL is "", an absolute http:// or https:// URL with a host, or a path
// on this server starting with a single "/", such as "/static/logo.svg".
// Pages link it from a browser, so a relative path or another scheme is
// refused.
type WebURL string

func (u *WebURL) UnmarshalText(text []byte) error {
	s := string(text)
	if s != "" && !isWebURL(s) {
		return &ConfigError{
			Reason: `is not an http:// or https:// URL or a path starting with "/"`,
			Hint:   `write a path on this server, e.g. "/static/logo.svg", or a full URL, e.g. "https://cdn.example.com/brand.css"`,
		}
	}

	*u = WebURL(s)

	return nil
}

func isWebURL(s string) bool {
	p, err := url.Parse(s)
	if err != nil {
		return false
	}

	if strings.HasPrefix(s, "/") {
		// "//host/x" is protocol-relative and "/\host" is read as one by
		// browsers: both leave this server.
		return p.Scheme == "" && p.Host == "" && !strings.HasPrefix(s, "//") && !strings.HasPrefix(s, `/\`)
	}

	return (p.Scheme == "http" || p.Scheme == "https") && p.Hostname() != ""
}

// CSSColor is "" or a CSS hex colour: #rgb, #rgba, #rrggbb or #rrggbbaa.
// Only hex is accepted so the value can be written into a stylesheet
// as is.
type CSSColor string

func (c *CSSColor) UnmarshalText(text []byte) error {
	s := string(text)
	if s != "" && !isHexColor(s) {
		return &ConfigError{
			Reason: "is not a hex colour",
			Hint:   `write it as "#rrggbb", e.g. "#4051b5"`,
		}
	}

	*c = CSSColor(s)

	return nil
}

func isHexColor(s string) bool {
	hex, ok := strings.CutPrefix(s, "#")
	if !ok {
		return false
	}

	switch len(hex) {
	case 3, 4, 6, 8:
	default:
		return false
	}

	for _, r := range hex {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}

	return true
}

// FontFamily is "" or a CSS font-family list, such as `"Inter", sans-serif`:
// letters, digits, spaces, commas, hyphens, underscores and balanced
// quotes. Anything else, such as ";" or "}", is refused so the value can be
// written into a stylesheet as is.
type FontFamily string

func (f *FontFamily) UnmarshalText(text []byte) error {
	s := string(text)
	if s != "" && !isFontFamily(s) {
		return &ConfigError{
			Reason: "is not a CSS font list",
			Hint:   `use font names, quoted when they contain spaces, separated by commas, e.g. "\"Inter\", sans-serif"`,
		}
	}

	*f = FontFamily(s)

	return nil
}

func isFontFamily(s string) bool {
	for _, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			strings.ContainsRune(` ,-_"'`, r)
		if !ok {
			return false
		}
	}

	return strings.Count(s, `"`)%2 == 0 && strings.Count(s, `'`)%2 == 0
}

// ListenAddr is "" or a TCP host:port address; the port may be "http" or
// "https".
type ListenAddr string

func (a *ListenAddr) UnmarshalText(text []byte) error {
	s := string(text)
	if s != "" {
		_, err := PortFromAddr(s)
		if err != nil {
			return &ConfigError{
				Reason: "is not a host:port address",
				Detail: err.Error(),
				Hint:   `use host:port form, e.g. "0.0.0.0:8080"`,
				Cause:  err,
			}
		}
	}

	*a = ListenAddr(s)

	return nil
}

// setEnum stores s in dst when it is "" (unset) or one of allowed;
// otherwise it returns e completed with the allowed values.
func setEnum[T ~string](dst *T, s string, allowed []string, e ConfigError) error {
	if s != "" && !slices.Contains(allowed, s) {
		e.Reason = "has an unsupported value"
		e.Allowed = allowed

		return &e
	}

	*dst = T(s)

	return nil
}

// LogLevel is a zerolog level name, case-insensitive and stored lowercase.
// "" decodes without error to the zero value (unset).
type LogLevel string //nolint:recvcheck // UnmarshalText needs a pointer; the rest read the value

func (LogLevel) Allowed() []string {
	return []string{"trace", "debug", "info", "warn", "error", "fatal", "panic", "disabled"}
}

func (l *LogLevel) UnmarshalText(text []byte) error {
	return setEnum(l, strings.ToLower(string(text)), l.Allowed(), ConfigError{})
}

// Zerolog returns the level: info when unset (zerolog's NoLevel would
// hide every line); one zerolog does not know is debug.
func (l LogLevel) Zerolog() zerolog.Level {
	if l == "" {
		return zerolog.InfoLevel
	}

	lvl, err := zerolog.ParseLevel(string(l))
	if err != nil {
		return zerolog.DebugLevel
	}

	return lvl
}

// LogFormat is the log output format. "" decodes without error to the zero
// value (unset).
type LogFormat string //nolint:recvcheck // UnmarshalText needs a pointer; the rest read the value

func (LogFormat) Allowed() []string { return []string{string(TextLogFormat), string(JSONLogFormat)} }

func (f *LogFormat) UnmarshalText(text []byte) error {
	return setEnum(f, string(text), f.Allowed(), ConfigError{})
}

// DatabaseType is the database backend. "" decodes without error to the
// zero value (unset).
type DatabaseType string //nolint:recvcheck // UnmarshalText needs a pointer; the rest read the value

func (DatabaseType) Allowed() []string {
	return []string{string(DatabaseSqliteShort), string(DatabaseSqlite), string(DatabasePostgres)}
}

func (d *DatabaseType) UnmarshalText(text []byte) error {
	return setEnum(d, string(text), d.Allowed(), ConfigError{
		Hint: "pick one of the allowed values; sqlite is the default for single-host deployments",
		See:  "https://headscale.net/stable/ref/database/",
	})
}

// ACMEChallenge is the ACME challenge type. "" decodes without error to the
// zero value (unset).
type ACMEChallenge string //nolint:recvcheck // UnmarshalText needs a pointer; the rest read the value

func (ACMEChallenge) Allowed() []string {
	return []string{string(HTTP01ChallengeType), string(TLSALPN01ChallengeType), string(DNS01ChallengeType)}
}

func (c *ACMEChallenge) UnmarshalText(text []byte) error {
	return setEnum(c, string(text), c.Allowed(), ConfigError{
		Hint: "pick one of the allowed values; HTTP-01 is the default",
	})
}

// PKCEMethod is the OIDC PKCE challenge method. "" decodes without error to
// the zero value (unset).
type PKCEMethod string //nolint:recvcheck // UnmarshalText needs a pointer; the rest read the value

func (PKCEMethod) Allowed() []string {
	return []string{string(PKCEMethodPlain), string(PKCEMethodS256)}
}

func (p *PKCEMethod) UnmarshalText(text []byte) error {
	return setEnum(p, string(text), p.Allowed(), ConfigError{Cause: errInvalidPKCEMethod})
}

func (PolicyMode) Allowed() []string { return []string{string(PolicyModeFile), string(PolicyModeDB)} }

func (m *PolicyMode) UnmarshalText(text []byte) error {
	return setEnum(m, string(text), m.Allowed(), ConfigError{
		See: "https://headscale.net/stable/ref/policy/",
	})
}

func (IPAllocationStrategy) Allowed() []string {
	return []string{string(IPAllocationStrategySequential), string(IPAllocationStrategyRandom)}
}

func (s *IPAllocationStrategy) UnmarshalText(text []byte) error {
	return setEnum(s, string(text), s.Allowed(), ConfigError{
		Hint:  "pick one of the allowed values; sequential is the default",
		Cause: ErrInvalidAllocationStrategy,
	})
}
