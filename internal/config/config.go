// Package config loads and validates the process's typed configuration.
//
// Precedence is file, then APP_-prefixed environment variables, then flags.
// Validate is the single place that rejects a configuration the process
// cannot safely run under, so startup fails at boot rather than at first use.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/sujanto-gaws/kopiochi/internal/platform/secret"
)

type Config struct {
	Server `mapstructure:"server"`
	DB     `mapstructure:"db"`
	Log    `mapstructure:"log"`
	Auth   `mapstructure:"auth"`
	// Security is a named field rather than an embedded struct: it carries
	// nested sections (CORS, RateLimit) whose field names would otherwise be
	// promoted into Config and collide as the config grows. See
	// docs/architectures/03-configuration/configuration-model.md.
	Security Security `mapstructure:"security"`
	// Metrics is named for the same reason as Security.
	Metrics Metrics `mapstructure:"metrics"`
}

// Metrics configures the Prometheus scrape endpoint.
//
// It listens on its own address, separate from the API. /metrics exposes the
// route table, latency distributions, pool saturation and the process's
// memory and file-descriptor counts — an inventory of the service's internals
// that has no business being reachable from the internet. The default binds
// loopback only, so exposing it is a deliberate act rather than the result of
// enabling a feature.
type Metrics struct {
	Enabled bool   `mapstructure:"enabled"`
	Addr    string `mapstructure:"addr"`
	Path    string `mapstructure:"path"`
}

type Server struct {
	Host              string        `mapstructure:"host"`
	Port              int           `mapstructure:"port"`
	ReadTimeout       time.Duration `mapstructure:"read_timeout"`
	ReadHeaderTimeout time.Duration `mapstructure:"read_header_timeout"`
	WriteTimeout      time.Duration `mapstructure:"write_timeout"`
	IdleTimeout       time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout   time.Duration `mapstructure:"shutdown_timeout"`
	RequestTimeout    time.Duration `mapstructure:"request_timeout"`
	// TrustedProxies lists CIDR ranges whose forwarded headers
	// (X-Forwarded-For, X-Real-Ip) are honored when resolving the client
	// IP. The default is empty, meaning trust nothing: the socket address
	// is always used. See internal/middleware.RealIP.
	TrustedProxies []string `mapstructure:"trusted_proxies"`
	// EnableHSTS gates the Strict-Transport-Security response header
	// (internal/httpx.SecurityHeaders). It must only be turned on when TLS
	// is actually terminated somewhere in front of this process -- this
	// server always listens plain HTTP. Emitting HSTS unconditionally would,
	// at best, be ignored by a client talking plain HTTP and, at worst, get
	// cached by a browser against a plain-HTTP dev origin such as
	// http://localhost and lock a developer out of it. Default: false.
	EnableHSTS bool `mapstructure:"enable_hsts"`
}

type DB struct {
	Host     string        `mapstructure:"host"`
	Port     int           `mapstructure:"port"`
	User     string        `mapstructure:"user"`
	Password secret.String `mapstructure:"password"`
	Name     string        `mapstructure:"name"`
	SSLMode  string        `mapstructure:"sslmode"`
	MaxConns int32         `mapstructure:"max_conns"`
	MinConns int32         `mapstructure:"min_conns"`
	// ConnMaxLifetime and ConnMaxIdleTime were hardcoded in internal/db
	// while MaxConns/MinConns were configurable — an arbitrary split that
	// made the two settings most likely to need tuning under load the two
	// that required a rebuild. Both now apply to the pgx pool and to the
	// sql.DB wrapper on top of it.
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
	// HealthCheckPeriod is how often the pool checks idle connections for
	// liveness, so a connection severed by a firewall or failover is
	// discovered by the pool rather than by a request.
	HealthCheckPeriod time.Duration `mapstructure:"health_check_period"`
	// ConnectTimeout bounds a single dial. Without it, a network partition
	// makes every connection attempt hang instead of failing fast — at
	// startup and, worse, on the request path afterwards.
	ConnectTimeout time.Duration `mapstructure:"connect_timeout"`
	// StartupTimeout bounds the whole open-and-ping sequence at boot. It is
	// separate from ConnectTimeout because it covers pool construction plus
	// the verification ping, not one dial.
	StartupTimeout time.Duration `mapstructure:"startup_timeout"`
}

type Log struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type Auth struct {
	PrivateKeyPath    string        `mapstructure:"private_key_path"`
	PublicKeyPath     string        `mapstructure:"public_key_path"`
	Issuer            string        `mapstructure:"issuer"`
	ClientID          string        `mapstructure:"client_id"`
	AccessTokenTTL    time.Duration `mapstructure:"access_token_ttl"`
	RefreshTokenTTL   time.Duration `mapstructure:"refresh_token_ttl"`
	MFATemporaryTTL   time.Duration `mapstructure:"mfa_temporary_ttl"`
	MaxFailedAttempts int           `mapstructure:"max_failed_attempts"`
	LockDuration      time.Duration `mapstructure:"lock_duration"`
	// TokenLeeway is the clock-skew allowance applied when validating a
	// token's exp. Kept small and non-zero (see
	// docs/architectures/04-security/token-architecture.md).
	TokenLeeway time.Duration `mapstructure:"token_leeway"`
}

// Security holds the cross-cutting HTTP concerns that used to be configured
// through the generic plugin map (plugins.middleware / plugins.custom). They
// are ordinary typed configuration, not business modules, so they are
// constructed directly in internal/httpx rather than registered in a plugin
// framework -- see
// docs/architectures/01-modularity/extension-framework.md, "What about
// genuinely optional middleware?".
//
// Because the fields are typed, a YAML type error (requests: "500") is now a
// startup failure instead of a silently-applied default, which was defect 3
// of the plugin config contract.
type Security struct {
	CORS      CORS      `mapstructure:"cors"`
	RateLimit RateLimit `mapstructure:"rate_limit"`
}

// CORS configures the Cross-Origin Resource Sharing middleware
// (internal/httpx.CORS). It is allowlist-only and deny-by-default: an empty
// AllowedOrigins grants no origin access, and a wildcard must be listed
// explicitly as "*".
type CORS struct {
	// Enabled gates the middleware entirely. Disabled (the default) means
	// no CORS headers are ever emitted -- which, for a same-origin or
	// non-browser API, is the correct posture.
	Enabled          bool          `mapstructure:"enabled"`
	AllowedOrigins   []string      `mapstructure:"allowed_origins"`
	AllowedMethods   []string      `mapstructure:"allowed_methods"`
	AllowedHeaders   []string      `mapstructure:"allowed_headers"`
	AllowCredentials bool          `mapstructure:"allow_credentials"`
	MaxAge           time.Duration `mapstructure:"max_age"`
}

// RateLimit configures the token-bucket rate limiter
// (internal/httpx.NewRateLimiter).
type RateLimit struct {
	Enabled bool `mapstructure:"enabled"`
	// Rate is the sustained allowance in requests per minute.
	Rate float64 `mapstructure:"rate"`
	// Burst is the bucket capacity: the instantaneous allowance a key may
	// spend before it is throttled to Rate.
	Burst float64 `mapstructure:"burst"`
	// TTL is how long an idle bucket is kept before the sweep evicts it.
	TTL time.Duration `mapstructure:"ttl"`
	// MaxKeys caps the number of tracked keys. New keys are rejected once
	// the table is full rather than evicting an existing bucket, which
	// would be gameable -- see internal/httpx/ratelimit.go.
	MaxKeys int `mapstructure:"max_keys"`
}

func Load(cfgPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(cfgPath)
	v.SetEnvPrefix("APP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Secret keys are intentionally absent from the YAML config files.
	// AutomaticEnv() alone does not surface env vars to Unmarshal() unless
	// Viper already knows the key (via config file, SetDefault, or BindEnv),
	// so these must be bound explicitly before ReadInConfig/Unmarshal.
	if err := v.BindEnv("db.password", "APP_DB_PASSWORD"); err != nil {
		return nil, fmt.Errorf("bind db.password env: %w", err)
	}
	// db.user and db.name have the same defect as db.password did: no
	// registered default and no BindEnv, so APP_DB_USER/APP_DB_NAME are
	// silently ignored unless the corresponding key also appears in the
	// YAML file. Bind them explicitly for the same reason.
	if err := v.BindEnv("db.user", "APP_DB_USER"); err != nil {
		return nil, fmt.Errorf("bind db.user env: %w", err)
	}
	if err := v.BindEnv("db.name", "APP_DB_NAME"); err != nil {
		return nil, fmt.Errorf("bind db.name env: %w", err)
	}

	// Defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.read_header_timeout", "10s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.idle_timeout", "120s")
	v.SetDefault("server.shutdown_timeout", "30s")
	// Must not exceed write_timeout (see Config.Validate and
	// config/default.yaml).
	v.SetDefault("server.request_timeout", "25s")
	// Empty by default: trust no proxy, always use the socket address for
	// the client IP. A permissive default here would be a security bug.
	v.SetDefault("server.trusted_proxies", []string{})
	// Off by default: this server always listens plain HTTP. Only flip this
	// on for a deployment where TLS is genuinely terminated in front of it.
	v.SetDefault("server.enable_hsts", false)
	v.SetDefault("db.host", "localhost")
	v.SetDefault("db.port", 5432)
	v.SetDefault("db.sslmode", "disable")
	v.SetDefault("db.max_conns", 10)
	v.SetDefault("db.min_conns", 2)
	// The two lifetimes carry forward the values internal/db used to
	// hardcode, so this change is configurability, not a behaviour change.
	v.SetDefault("db.conn_max_lifetime", "1h")
	v.SetDefault("db.conn_max_idle_time", "30m")
	v.SetDefault("db.health_check_period", "1m")
	v.SetDefault("db.connect_timeout", "5s")
	v.SetDefault("db.startup_timeout", "15s")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
	v.SetDefault("auth.private_key_path", "keys/private.pem")
	v.SetDefault("auth.public_key_path", "keys/public.pem")
	v.SetDefault("auth.issuer", "kopiochi")
	v.SetDefault("auth.client_id", "kopiochi")
	v.SetDefault("auth.access_token_ttl", "15m")
	v.SetDefault("auth.refresh_token_ttl", "168h")
	v.SetDefault("auth.mfa_temporary_ttl", "5m")
	v.SetDefault("auth.max_failed_attempts", 5)
	v.SetDefault("auth.lock_duration", "15m")
	v.SetDefault("auth.token_leeway", "30s")
	// Both cross-cutting middlewares default to off. Enabling CORS without
	// an allowlist still grants nothing (deny-by-default), and enabling the
	// rate limiter uses the tunables below.
	v.SetDefault("security.cors.enabled", false)
	v.SetDefault("security.cors.allowed_origins", []string{})
	v.SetDefault("security.cors.allowed_methods", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"})
	v.SetDefault("security.cors.allowed_headers", []string{"Accept", "Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization"})
	v.SetDefault("security.cors.allow_credentials", false)
	v.SetDefault("security.cors.max_age", "5m")
	v.SetDefault("security.rate_limit.enabled", false)
	v.SetDefault("security.rate_limit.rate", 100)
	v.SetDefault("security.rate_limit.burst", 100)
	v.SetDefault("security.rate_limit.ttl", "10m")
	v.SetDefault("security.rate_limit.max_keys", 100000)
	// Off by default, and loopback-only when switched on: /metrics describes
	// the service's internals and must be an explicit exposure decision.
	v.SetDefault("metrics.enabled", false)
	v.SetDefault("metrics.addr", "127.0.0.1:9090")
	v.SetDefault("metrics.path", "/metrics")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// placeholderPrefix flags every "CHANGEME*"-style value shipped in
// .env.example. A prefix check is more durable than an exact-string
// blacklist: it still catches the placeholder if someone tweaks the
// trailing wording without actually generating a real secret.
const placeholderPrefix = "CHANGEME"

// legacyPlaceholderSecrets are specific values that were, at one point,
// committed to the repository as if they were real credentials (see
// docs/architectures/03-configuration/secret-management.md). They are
// rejected by exact match because they are historical facts, not a naming
// convention like placeholderPrefix.
var legacyPlaceholderSecrets = map[string]bool{
	"postgres":                             true,
	"your-secret-key-change-in-production": true,
}

// isPlaceholderSecret reports whether v is empty, a "CHANGEME*" placeholder,
// or one of the historical committed values. It is only ever applied to
// values that must hold a real secret (e.g. a DB password) — never to
// non-secret fields such as db.user, where "postgres" is a perfectly
// ordinary value.
func isPlaceholderSecret(v string) bool {
	if v == "" {
		return true
	}
	if strings.HasPrefix(strings.ToUpper(v), placeholderPrefix) {
		return true
	}
	return legacyPlaceholderSecrets[v]
}

// Validate rejects configuration that would otherwise fail later, at first
// request or first DB connection, instead of at startup. Load calls this so
// the process refuses to start rather than serve traffic against a broken
// or insecure configuration.
func (c *Config) Validate() error {
	var errs []error

	// The CORS spec forbids combining a wildcard allowed origin with
	// credentialed requests, and browsers ignore Access-Control-Allow-
	// Credentials in that combination anyway -- but failing loudly here, at
	// startup, is much better than silently serving a CORS config that
	// looks like it grants credentialed cross-origin access and does not
	// (or, in a browser that gets it wrong, does). See
	// docs/architectures/04-security/middleware-hardening.md, Problem 2.
	if c.Security.CORS.AllowCredentials {
		for _, o := range c.Security.CORS.AllowedOrigins {
			if o == "*" {
				errs = append(errs, errors.New(`security.cors: allowed_origins "*" cannot be combined with allow_credentials`))
				break
			}
		}
	}

	// The rate limiter's tunables are only meaningful when it is on, but a
	// nonsensical value in a disabled block is still a configuration
	// mistake worth surfacing at boot rather than at the moment someone
	// flips enabled: true in production.
	if c.Security.RateLimit.Enabled {
		if c.Security.RateLimit.Rate <= 0 {
			errs = append(errs, errors.New("security.rate_limit.rate must be positive"))
		}
		if c.Security.RateLimit.Burst <= 0 {
			errs = append(errs, errors.New("security.rate_limit.burst must be positive"))
		}
		if c.Security.RateLimit.TTL <= 0 {
			errs = append(errs, errors.New("security.rate_limit.ttl must be positive"))
		}
		if c.Security.RateLimit.MaxKeys <= 0 {
			errs = append(errs, errors.New("security.rate_limit.max_keys must be positive"))
		}
	}

	if c.DB.Name == "" {
		errs = append(errs, errors.New("db.name is required (set APP_DB_NAME)"))
	}
	if c.DB.User == "" {
		errs = append(errs, errors.New("db.user is required (set APP_DB_USER)"))
	}
	if c.DB.MinConns > c.DB.MaxConns {
		errs = append(errs, errors.New("db.min_conns must not exceed db.max_conns"))
	}
	if c.DB.MaxConns <= 0 {
		errs = append(errs, errors.New("db.max_conns must be positive"))
	}
	if c.DB.ConnMaxLifetime <= 0 {
		errs = append(errs, errors.New("db.conn_max_lifetime must be positive"))
	}
	if c.DB.ConnMaxIdleTime <= 0 {
		errs = append(errs, errors.New("db.conn_max_idle_time must be positive"))
	}
	// An idle connection kept longer than its total lifetime is a
	// contradiction: the lifetime cap fires first and the idle setting can
	// never take effect, which reads as a tuning knob that silently does
	// nothing.
	if c.DB.ConnMaxIdleTime > c.DB.ConnMaxLifetime {
		errs = append(errs, fmt.Errorf(
			"db.conn_max_idle_time (%s) must not exceed db.conn_max_lifetime (%s)",
			c.DB.ConnMaxIdleTime, c.DB.ConnMaxLifetime))
	}
	if c.DB.ConnectTimeout <= 0 {
		errs = append(errs, errors.New("db.connect_timeout must be positive"))
	}
	// The startup budget covers pool construction plus a verification ping,
	// each of which can spend a full dial; a budget below one dial would
	// abort boot before a single connection attempt could finish.
	if c.DB.StartupTimeout < c.DB.ConnectTimeout {
		errs = append(errs, fmt.Errorf(
			"db.startup_timeout (%s) must be >= db.connect_timeout (%s)",
			c.DB.StartupTimeout, c.DB.ConnectTimeout))
	}
	if isPlaceholderSecret(c.DB.Password.Reveal()) {
		errs = append(errs, errors.New("db.password is empty or a known placeholder; set APP_DB_PASSWORD to a real credential"))
	}

	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		errs = append(errs, fmt.Errorf("server.port (%d) must be between 1 and 65535", c.Server.Port))
	}
	// The write deadline must not fire before the request-handling deadline,
	// or a slow-but-legitimate request gets its connection severed mid
	// response instead of completing or timing out cleanly. See
	// docs/architectures/03-configuration/configuration-model.md.
	if c.Server.RequestTimeout > c.Server.WriteTimeout {
		errs = append(errs, fmt.Errorf(
			"server.request_timeout (%s) must not exceed server.write_timeout (%s)",
			c.Server.RequestTimeout, c.Server.WriteTimeout))
	}
	if c.Server.ShutdownTimeout < c.Server.RequestTimeout {
		errs = append(errs, errors.New("server.shutdown_timeout must be >= server.request_timeout"))
	}

	if c.Metrics.Enabled {
		if c.Metrics.Addr == "" {
			errs = append(errs, errors.New("metrics.addr must not be empty when metrics.enabled is true"))
		}
		if !strings.HasPrefix(c.Metrics.Path, "/") {
			errs = append(errs, fmt.Errorf(
				"metrics.path (%q) must begin with '/'", c.Metrics.Path))
		}
		// Same port as the API would put /metrics — the pool sizes, the route
		// table, the process's memory and fd counts — on the public listener,
		// which is the one thing a separate admin port exists to prevent.
		if c.Metrics.Addr == fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port) {
			errs = append(errs, fmt.Errorf(
				"metrics.addr (%s) must not be the API's own address; /metrics belongs on a separate, non-public listener",
				c.Metrics.Addr))
		}
	}

	if c.Auth.Issuer == "" {
		errs = append(errs, errors.New("auth.issuer must not be empty"))
	}
	if c.Auth.AccessTokenTTL <= 0 {
		errs = append(errs, errors.New("auth.access_token_ttl must be positive"))
	}
	if c.Auth.RefreshTokenTTL <= 0 {
		errs = append(errs, errors.New("auth.refresh_token_ttl must be positive"))
	}
	if c.Auth.AccessTokenTTL > 0 && c.Auth.RefreshTokenTTL > 0 && c.Auth.AccessTokenTTL >= c.Auth.RefreshTokenTTL {
		errs = append(errs, errors.New("auth.access_token_ttl must be shorter than auth.refresh_token_ttl"))
	}
	if c.Auth.MFATemporaryTTL <= 0 {
		errs = append(errs, errors.New("auth.mfa_temporary_ttl must be positive"))
	}
	if c.Auth.LockDuration <= 0 {
		errs = append(errs, errors.New("auth.lock_duration must be positive"))
	}
	if c.Auth.MaxFailedAttempts <= 0 {
		errs = append(errs, errors.New("auth.max_failed_attempts must be positive"))
	}
	if c.Auth.TokenLeeway < 0 {
		errs = append(errs, errors.New("auth.token_leeway must not be negative"))
	}

	return errors.Join(errs...)
}
