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
	Server  `mapstructure:"server"`
	DB      `mapstructure:"db"`
	Log     `mapstructure:"log"`
	Auth    `mapstructure:"auth"`
	Plugins `mapstructure:"plugins"`
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

type Plugins struct {
	Middleware []string                          `mapstructure:"middleware"`
	Auth       map[string]PluginAuthConfig       `mapstructure:"auth"`
	Cache      map[string]PluginCacheConfig      `mapstructure:"cache"`
	Custom     map[string]map[string]interface{} `mapstructure:"custom"`
}

type PluginAuthConfig struct {
	Enabled  bool                   `mapstructure:"enabled"`
	Provider string                 `mapstructure:"provider"`
	Config   map[string]interface{} `mapstructure:"config"`
}

type PluginCacheConfig struct {
	Enabled  bool                   `mapstructure:"enabled"`
	Provider string                 `mapstructure:"provider"`
	Config   map[string]interface{} `mapstructure:"config"`
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
	if err := v.BindEnv("plugins.auth.jwt.config.secret", "APP_JWT_SECRET"); err != nil {
		return nil, fmt.Errorf("bind plugins.auth.jwt.config.secret env: %w", err)
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
	v.SetDefault("plugins.middleware", []string{})
	v.SetDefault("plugins.auth", map[string]interface{}{})
	v.SetDefault("plugins.cache", map[string]interface{}{})
	v.SetDefault("plugins.custom", map[string]interface{}{})

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
// values that must hold a real secret (a DB password, an HMAC/JWT secret) —
// never to non-secret fields such as db.user, where "postgres" is a
// perfectly ordinary value.
func isPlaceholderSecret(v string) bool {
	if v == "" {
		return true
	}
	if strings.HasPrefix(strings.ToUpper(v), placeholderPrefix) {
		return true
	}
	return legacyPlaceholderSecrets[v]
}

// jwtPluginSecret extracts the legacy HS256 jwt-auth plugin's secret from
// the generic plugins.auth config map (internal/plugins/auth/jwt.go). The
// plugin config surface is still map[string]interface{} (see
// docs/architectures/03-configuration/configuration-model.md, "Problem 3");
// reworking that into a typed struct is out of scope here. The bool return
// tells the caller whether the plugin is actually enabled, so an unused,
// unconfigured plugin entry (the shipped default) does not fail validation.
func jwtPluginSecret(c *Config) (string, bool) {
	jwtCfg, ok := c.Plugins.Auth["jwt"]
	if !ok || !jwtCfg.Enabled {
		return "", false
	}
	s, _ := jwtCfg.Config["secret"].(string)
	return s, true
}

// corsAllowedOriginsAndCredentials extracts the "cors" middleware plugin's
// allowed_origins/allow_credentials from the generic plugins.custom map
// (internal/plugins/middleware/cors.go's config surface is still
// map[string]interface{}; see jwtPluginSecret above for the same pattern).
// A missing "cors" section returns no origins and no credentials -- the
// same safe, deny-everything state the plugin itself defaults to.
func corsAllowedOriginsAndCredentials(c *Config) (origins []string, allowCredentials bool) {
	cors, ok := c.Plugins.Custom["cors"]
	if !ok {
		return nil, false
	}
	if raw, ok := cors["allowed_origins"].([]interface{}); ok {
		for _, o := range raw {
			if s, ok := o.(string); ok {
				origins = append(origins, s)
			}
		}
	}
	if b, ok := cors["allow_credentials"].(bool); ok {
		allowCredentials = b
	}
	return origins, allowCredentials
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
	if origins, allowCreds := corsAllowedOriginsAndCredentials(c); allowCreds {
		for _, o := range origins {
			if o == "*" {
				errs = append(errs, errors.New(`plugins.custom.cors: allowed_origins "*" cannot be combined with allow_credentials`))
				break
			}
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
	if isPlaceholderSecret(c.DB.Password.Reveal()) {
		errs = append(errs, errors.New("db.password is empty or a known placeholder; set APP_DB_PASSWORD to a real credential"))
	}
	if s, enabled := jwtPluginSecret(c); enabled && isPlaceholderSecret(s) {
		errs = append(errs, errors.New("plugins.auth.jwt.config.secret is empty or a known placeholder; set APP_JWT_SECRET to a real secret"))
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
