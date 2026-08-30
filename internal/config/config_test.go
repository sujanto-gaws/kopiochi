package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// validYAML is a config file that satisfies Config.Validate() end to end
// (real db.name/db.user, a non-placeholder db.password, and timeouts that
// don't invert), so tests that aren't specifically exercising a validation
// failure can start from something known-good and mutate one thing.
const validYAML = `
db:
  host: "localhost"
  port: 5432
  user: "postgres"
  password: "a-real-dev-password"
  name: "kopiochi"
  sslmode: "disable"
`

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLoad_DBPasswordEnvFallback guards against a Viper trap: AutomaticEnv()
// alone does not surface an env var to Unmarshal() unless the key is already
// known to Viper (via config file, SetDefault, or BindEnv). Since
// db.password is intentionally absent from the YAML config, it must be
// reachable only via an explicit BindEnv + APP_DB_PASSWORD.
func TestLoad_DBPasswordEnvFallback(t *testing.T) {
	cfgPath := writeConfig(t, `
db:
  host: "localhost"
  port: 5432
  user: "postgres"
  name: "kopiochi"
  sslmode: "disable"
`)

	t.Run("env set", func(t *testing.T) {
		t.Setenv("APP_DB_PASSWORD", "secret-from-env")
		cfg, err := Load(cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.DB.Password.Reveal(); got != "secret-from-env" {
			t.Errorf("cfg.DB.Password.Reveal() = %q, want %q", got, "secret-from-env")
		}
	})

	t.Run("env unset", func(t *testing.T) {
		// db.password has no default and no file value here, so it decodes
		// to the empty string — which Validate now rejects outright rather
		// than silently starting the process with no credential (Phase 2.9
		// fail-closed behavior). This supersedes the pre-2.9 assertion that
		// Load succeeded with an empty password.
		_, err := Load(cfgPath)
		if err == nil {
			t.Fatal("expected Load to fail closed on an empty db.password, got nil error")
		}
		if !strings.Contains(err.Error(), "db.password") {
			t.Errorf("error = %v, want it to mention db.password", err)
		}
	})
}

// TestLoad_EnvOverridesFileValue extends the fallback coverage above by
// proving APP_DB_PASSWORD wins even when the config file already sets
// db.password — the test above only ever exercises the "key absent from
// file" case (env as the only source), never "env overrides an existing
// file value", which is the actual scenario the remediation plan's
// TestEnvOverridesFile sketch (testing-strategy.md) describes.
func TestLoad_EnvOverridesFileValue(t *testing.T) {
	cfgPath := writeConfig(t, `
db:
  host: "localhost"
  port: 5432
  user: "postgres"
  password: "from-file"
  name: "kopiochi"
  sslmode: "disable"
`)

	t.Setenv("APP_DB_PASSWORD", "from-env")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.DB.Password.Reveal(); got != "from-env" {
		t.Errorf("cfg.DB.Password.Reveal() = %q, want %q (env must override a value already present in the file)", got, "from-env")
	}
}

// TestLoad_DBUserAndNameEnvFallback is the same class of defect as
// TestLoad_DBPasswordEnvFallback, but for db.user/db.name
// (configuration-model.md: neither has a default nor a BindEnv, so
// APP_DB_USER/APP_DB_NAME were silently ignored whenever the key wasn't
// already present in the YAML file).
func TestLoad_DBUserAndNameEnvFallback(t *testing.T) {
	// Deliberately omit user/name from the file so the only way they can be
	// set is via env — the failure mode this guards against is Viper
	// silently ignoring APP_DB_USER/APP_DB_NAME because the keys were never
	// registered.
	cfgPath := writeConfig(t, `
db:
  host: "localhost"
  port: 5432
  password: "a-real-dev-password"
  sslmode: "disable"
`)

	t.Setenv("APP_DB_USER", "app_user_from_env")
	t.Setenv("APP_DB_NAME", "app_db_from_env")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.User != "app_user_from_env" {
		t.Errorf("cfg.DB.User = %q, want %q", cfg.DB.User, "app_user_from_env")
	}
	if cfg.DB.Name != "app_db_from_env" {
		t.Errorf("cfg.DB.Name = %q, want %q", cfg.DB.Name, "app_db_from_env")
	}
}

// TestLoad_RejectsPlaceholderSecrets is the Phase 2 exit criterion: a
// deployment carrying a copy-pasted .env.example value (or one of the
// historical committed placeholders) must refuse to start rather than run
// with a forgeable/known credential.
func TestLoad_RejectsPlaceholderSecrets(t *testing.T) {
	cases := []struct {
		name     string
		password string
		wantErr  string
	}{
		{
			name:     "shipped .env.example placeholder",
			password: "CHANGEME_SET_YOUR_OWN_DB_PASSWORD",
			wantErr:  "db.password",
		},
		{
			name:     "historical hardcoded value",
			password: "postgres",
			wantErr:  "db.password",
		},
		{
			name:     "empty",
			password: "",
			wantErr:  "db.password",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfgPath := writeConfig(t, `
db:
  host: "localhost"
  port: 5432
  user: "postgres"
  name: "kopiochi"
  sslmode: "disable"
`)
			if tc.password != "" {
				t.Setenv("APP_DB_PASSWORD", tc.password)
			}

			_, err := Load(cfgPath)
			if err == nil {
				t.Fatalf("Load succeeded with placeholder password %q; the server must refuse to start", tc.password)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

// TestLoad_ValidConfigStarts confirms the flip side of every rejection test
// above: a config with a real password and sane values loads without error.
// A Validate() that rejects a legitimately valid config would itself be an
// outage.
func TestLoad_ValidConfigStarts(t *testing.T) {
	cfgPath := writeConfig(t, validYAML)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed on a valid config: %v", err)
	}
	if cfg.DB.Password.Reveal() != "a-real-dev-password" {
		t.Errorf("cfg.DB.Password.Reveal() = %q, want %q", cfg.DB.Password.Reveal(), "a-real-dev-password")
	}
}

// TestLoad_DefaultYAMLIsValid guards against a self-inflicted outage: the
// committed config/default.yaml must itself satisfy Validate() once real
// secrets are supplied via env (as every deployment must do), otherwise the
// shipped defaults could never start.
func TestLoad_DefaultYAMLIsValid(t *testing.T) {
	t.Setenv("APP_DB_PASSWORD", "a-real-dev-password")

	if _, err := Load("../../config/default.yaml"); err != nil {
		t.Fatalf("Load(config/default.yaml) with a real APP_DB_PASSWORD failed: %v", err)
	}
}

func validConfig() *Config {
	return &Config{
		Server: Server{
			Port:            8080,
			RequestTimeout:  25 * time.Second,
			WriteTimeout:    30 * time.Second,
			ShutdownTimeout: 30 * time.Second,
		},
		DB: DB{
			User:            "postgres",
			Password:        "a-real-dev-password",
			Name:            "kopiochi",
			MaxConns:        10,
			MinConns:        2,
			ConnMaxLifetime: time.Hour,
			ConnMaxIdleTime: 30 * time.Minute,
			ConnectTimeout:  5 * time.Second,
			StartupTimeout:  15 * time.Second,
		},
		Auth: Auth{
			Issuer:            "kopiochi",
			AccessTokenTTL:    15 * time.Minute,
			RefreshTokenTTL:   168 * time.Hour,
			MFATemporaryTTL:   5 * time.Minute,
			MaxFailedAttempts: 5,
			LockDuration:      15 * time.Minute,
		},
	}
}

// TestValidate_AcceptsAValidConfig is the control for every rejection test
// below: it must pass, or the rejection tests would be meaningless.
func TestValidate_AcceptsAValidConfig(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("Validate() on a valid config returned an error: %v", err)
	}
}

// TestValidate_RejectsTimeoutInversion is the regression test for the
// concrete, currently-live defect: request_timeout (60s) exceeding
// write_timeout (30s) in config/default.yaml means the write deadline fires
// before the request-handling deadline, severing a slow-but-legitimate
// request's connection mid-response.
func TestValidate_RejectsTimeoutInversion(t *testing.T) {
	cfg := validConfig()
	cfg.Server.RequestTimeout = 60 * time.Second
	cfg.Server.WriteTimeout = 30 * time.Second

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() accepted request_timeout > write_timeout")
	}
	if !strings.Contains(err.Error(), "request_timeout") {
		t.Errorf("error = %v, want it to mention request_timeout", err)
	}
}

func TestValidate_RejectsEmptyDBNameAndUser(t *testing.T) {
	cfg := validConfig()
	cfg.DB.Name = ""
	cfg.DB.User = ""

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() accepted empty db.name/db.user")
	}
	if !strings.Contains(err.Error(), "db.name") || !strings.Contains(err.Error(), "db.user") {
		t.Errorf("error = %v, want it to mention both db.name and db.user", err)
	}
}

func TestValidate_RejectsNonPositiveTTLs(t *testing.T) {
	cfg := validConfig()
	cfg.Auth.AccessTokenTTL = 0
	cfg.Auth.RefreshTokenTTL = -1 * time.Second

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted a non-positive access/refresh token TTL")
	}
}

func TestValidate_RejectsEmptyIssuer(t *testing.T) {
	cfg := validConfig()
	cfg.Auth.Issuer = ""

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() accepted an empty auth.issuer")
	}
	if !strings.Contains(err.Error(), "issuer") {
		t.Errorf("error = %v, want it to mention issuer", err)
	}
}

func TestValidate_RejectsOutOfRangePort(t *testing.T) {
	for _, port := range []int{0, -1, 70000} {
		cfg := validConfig()
		cfg.Server.Port = port
		if err := cfg.Validate(); err == nil {
			t.Errorf("Validate() accepted out-of-range server.port = %d", port)
		}
	}
}

func TestValidate_RejectsMinConnsAboveMaxConns(t *testing.T) {
	cfg := validConfig()
	cfg.DB.MinConns = 20
	cfg.DB.MaxConns = 10

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted db.min_conns > db.max_conns")
	}
}

// TestValidate_RejectsWildcardCORSOriginWithCredentials is the config-load
// regression test for middleware-hardening.md, Problem 2: the CORS spec
// forbids combining a wildcard allowed origin with credentialed requests,
// and the server must refuse to start rather than let that combination
// reach request handling (internal/httpx/cors.go).
func TestValidate_RejectsWildcardCORSOriginWithCredentials(t *testing.T) {
	cfg := validConfig()
	cfg.Security.CORS = CORS{
		Enabled:          true,
		AllowedOrigins:   []string{"*"},
		AllowCredentials: true,
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() accepted security.cors allowed_origins \"*\" combined with allow_credentials: true")
	}
	if !strings.Contains(err.Error(), "cors") {
		t.Errorf("error = %v, want it to mention cors", err)
	}
}

// TestValidate_AllowsWildcardCORSOriginWithoutCredentials is the control for
// the rejection test above: a wildcard origin on its own (no credentials) is
// a legitimate, if permissive, deliberate choice and must not be rejected.
func TestValidate_AllowsWildcardCORSOriginWithoutCredentials(t *testing.T) {
	cfg := validConfig()
	cfg.Security.CORS = CORS{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() rejected a wildcard CORS origin without credentials: %v", err)
	}
}

// TestValidate_AllowsSpecificCORSOriginWithCredentials is the control
// proving the rejection above is specifically about the wildcard, not about
// allow_credentials in general: a named origin plus credentials is exactly
// the combination CORS supports.
func TestValidate_AllowsSpecificCORSOriginWithCredentials(t *testing.T) {
	cfg := validConfig()
	cfg.Security.CORS = CORS{
		Enabled:          true,
		AllowedOrigins:   []string{"https://app.example.com"},
		AllowCredentials: true,
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() rejected a specific CORS origin with credentials: %v", err)
	}
}

// TestLoad_RejectsWildcardCORSOriginWithCredentials proves the rejection
// happens end to end through Load (config file -> Unmarshal -> Validate),
// not just against a hand-built Config.
func TestLoad_RejectsWildcardCORSOriginWithCredentials(t *testing.T) {
	cfgPath := writeConfig(t, validYAML+`
security:
  cors:
    enabled: true
    allowed_origins: ["*"]
    allow_credentials: true
`)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load succeeded with security.cors allowed_origins \"*\" and allow_credentials: true")
	}
	if !strings.Contains(err.Error(), "cors") {
		t.Errorf("error = %v, want it to mention cors", err)
	}
}

// TestLoad_SecurityEnvOverrides proves the APP_SECURITY_* variables
// documented in .env.example are real: they reach the typed struct through
// Viper's replacer, including the comma-separated origin list, which relies
// on Viper's StringToSliceHookFunc to become a []string rather than a single
// string. The variables this replaced (APP_RATELIMIT_REQUESTS,
// APP_CORS_ALLOWED_ORIGINS) mapped to no Viper key at all and were silently
// ignored — the exact failure this test exists to prevent recurring.
func TestLoad_SecurityEnvOverrides(t *testing.T) {
	cfgPath := writeConfig(t, validYAML)

	t.Setenv("APP_SECURITY_CORS_ENABLED", "true")
	t.Setenv("APP_SECURITY_CORS_ALLOWED_ORIGINS", "https://a.example.com,https://b.example.com")
	t.Setenv("APP_SECURITY_RATE_LIMIT_ENABLED", "true")
	t.Setenv("APP_SECURITY_RATE_LIMIT_RATE", "250")
	t.Setenv("APP_SECURITY_RATE_LIMIT_TTL", "3m")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.Security.CORS.Enabled {
		t.Error("security.cors.enabled did not pick up APP_SECURITY_CORS_ENABLED")
	}
	want := []string{"https://a.example.com", "https://b.example.com"}
	got := cfg.Security.CORS.AllowedOrigins
	if len(got) != len(want) {
		t.Fatalf("allowed_origins = %#v, want %#v (comma-separated env list must split)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("allowed_origins[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if !cfg.Security.RateLimit.Enabled {
		t.Error("security.rate_limit.enabled did not pick up APP_SECURITY_RATE_LIMIT_ENABLED")
	}
	if cfg.Security.RateLimit.Rate != 250 {
		t.Errorf("rate = %v, want 250", cfg.Security.RateLimit.Rate)
	}
	if cfg.Security.RateLimit.TTL != 3*time.Minute {
		t.Errorf("ttl = %v, want 3m", cfg.Security.RateLimit.TTL)
	}
}

// TestValidate_MetricsChecksOnlyApplyWhenEnabled: the metrics block is off by
// default, and an unset addr must not fail a config that never listens.
func TestValidate_MetricsChecksOnlyApplyWhenEnabled(t *testing.T) {
	cfg := validConfig()
	cfg.Metrics = Metrics{Enabled: false, Addr: "", Path: ""}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil when metrics are disabled", err)
	}
}

func TestValidate_MetricsRejectsAnEmptyAddr(t *testing.T) {
	cfg := validConfig()
	cfg.Metrics = Metrics{Enabled: true, Addr: "", Path: "/metrics"}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "metrics.addr") {
		t.Errorf("Validate() = %v, want a metrics.addr error", err)
	}
}

func TestValidate_MetricsRejectsAPathWithoutASlash(t *testing.T) {
	cfg := validConfig()
	cfg.Metrics = Metrics{Enabled: true, Addr: "127.0.0.1:9090", Path: "metrics"}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "metrics.path") {
		t.Errorf("Validate() = %v, want a metrics.path error", err)
	}
}

// TestValidate_MetricsRefusesTheAPIsOwnAddress is the check worth having here.
// /metrics exposes the route table, pool sizes and process memory; serving it
// on the public listener is the one outcome a separate admin port exists to
// prevent, and it is a plausible mistake to make in a values file.
func TestValidate_MetricsRefusesTheAPIsOwnAddress(t *testing.T) {
	cfg := validConfig()
	cfg.Server.Host = "0.0.0.0"
	cfg.Server.Port = 8080
	cfg.Metrics = Metrics{Enabled: true, Addr: "0.0.0.0:8080", Path: "/metrics"}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "separate") {
		t.Errorf("Validate() = %v, want a refusal to serve /metrics on the API's own address", err)
	}
}

func TestValidate_MetricsAcceptsASeparateAddress(t *testing.T) {
	cfg := validConfig()
	cfg.Server.Host = "0.0.0.0"
	cfg.Server.Port = 8080
	cfg.Metrics = Metrics{Enabled: true, Addr: "127.0.0.1:9090", Path: "/metrics"}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

// The remaining Validate branches, each asserted individually. Validate
// accumulates every problem rather than returning the first, so a values file
// with three mistakes reports three — these confirm each branch is reachable
// and names the field it rejects.
func TestValidate_RejectsIndividualBadValues(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{"empty db name", func(c *Config) { c.DB.Name = "" }, "db.name"},
		{"empty db user", func(c *Config) { c.DB.User = "" }, "db.user"},
		{"port too low", func(c *Config) { c.Server.Port = 0 }, "server.port"},
		{"port too high", func(c *Config) { c.Server.Port = 70000 }, "server.port"},
		{"request timeout exceeds write timeout", func(c *Config) {
			c.Server.RequestTimeout = 60 * time.Second
			c.Server.WriteTimeout = 30 * time.Second
		}, "request_timeout"},
		{"shutdown shorter than request", func(c *Config) {
			c.Server.ShutdownTimeout = time.Second
			c.Server.RequestTimeout = 25 * time.Second
		}, "shutdown_timeout"},
		{"empty issuer", func(c *Config) { c.Auth.Issuer = "" }, "auth.issuer"},
		{"non-positive access ttl", func(c *Config) { c.Auth.AccessTokenTTL = 0 }, "access_token_ttl"},
		{"non-positive refresh ttl", func(c *Config) { c.Auth.RefreshTokenTTL = 0 }, "refresh_token_ttl"},
		{"access ttl not shorter than refresh", func(c *Config) {
			c.Auth.AccessTokenTTL = 200 * time.Hour
			c.Auth.RefreshTokenTTL = 168 * time.Hour
		}, "shorter"},
		{"non-positive mfa ttl", func(c *Config) { c.Auth.MFATemporaryTTL = 0 }, "mfa_temporary_ttl"},
		{"placeholder password", func(c *Config) { c.DB.Password = "postgres" }, "db.password"},
		{"empty password", func(c *Config) { c.DB.Password = "" }, "db.password"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			tc.mutate(cfg)

			err := cfg.Validate()
			if err == nil {
				t.Fatalf("Validate() = nil, want an error mentioning %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("Validate() = %v, want it to mention %q", err, tc.wantSub)
			}
		})
	}
}

// TestValidate_ReportsEveryProblemAtOnce: fixing a config one error per run is
// the difference between one edit and six.
func TestValidate_ReportsEveryProblemAtOnce(t *testing.T) {
	cfg := validConfig()
	cfg.DB.Name = ""
	cfg.DB.User = ""
	cfg.Auth.Issuer = ""

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want errors")
	}
	for _, want := range []string{"db.name", "db.user", "auth.issuer"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Validate() = %v, want it to also mention %q", err, want)
		}
	}
}

// TestDefaultYAMLLoads parses the config file the repository actually ships.
//
// Nothing else does: every other test builds a Config in code or uses a
// fixture under testdata/. So a typo in config/default.yaml — a mis-indented
// block, a duration written as a bare number — would be found by the first
// person to run the server, not by the suite.
//
// APP_DB_PASSWORD is set because Validate rejects the placeholder the sample
// file carries, which is the intended behaviour and not what this test is
// about.
func TestDefaultYAMLLoads(t *testing.T) {
	t.Setenv("APP_DB_PASSWORD", "a-real-dev-password")

	cfg, err := Load(filepath.Join("..", "..", "config", "default.yaml"))
	if err != nil {
		t.Fatalf("config/default.yaml does not load: %v", err)
	}

	// Spot-check a value from each section, so a block that silently failed to
	// map (the usual symptom of bad indentation) is caught rather than
	// defaulted over.
	if cfg.Server.Port == 0 {
		t.Error("server.port did not load")
	}
	if cfg.DB.MaxConns == 0 {
		t.Error("db.max_conns did not load")
	}
	if len(cfg.Security.CORS.AllowedMethods) == 0 {
		t.Error("security.cors.allowed_methods did not load")
	}
	if cfg.Security.RateLimit.MaxKeys == 0 {
		t.Error("security.rate_limit.max_keys did not load")
	}
	if cfg.Metrics.Addr == "" {
		t.Error("metrics.addr did not load")
	}
	if cfg.Metrics.Path != "/metrics" {
		t.Errorf("metrics.path = %q, want /metrics", cfg.Metrics.Path)
	}
}

// TestDefaultYAMLShipsBothMiddlewaresOff: CORS and the rate limiter default to
// off, and the sample must not quietly turn either on — an allowlist someone
// forgot to edit is worse than no CORS at all.
func TestDefaultYAMLShipsSafeDefaults(t *testing.T) {
	t.Setenv("APP_DB_PASSWORD", "a-real-dev-password")

	cfg, err := Load(filepath.Join("..", "..", "config", "default.yaml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.Security.CORS.Enabled {
		t.Error("config/default.yaml enables CORS")
	}
	if cfg.Security.RateLimit.Enabled {
		t.Error("config/default.yaml enables the rate limiter")
	}
	if cfg.Metrics.Enabled {
		t.Error("config/default.yaml enables the metrics listener")
	}
	if cfg.Server.EnableHSTS {
		t.Error("config/default.yaml enables HSTS, which is not safe over plain http in development")
	}
	if len(cfg.Server.TrustedProxies) != 0 {
		t.Errorf("config/default.yaml trusts proxies %v; the default must trust nothing", cfg.Server.TrustedProxies)
	}
}

// The notification section has more defaults than any other, and every one of
// them is a value the module refuses to run without. A key that silently fails
// to default is a boot failure with a confusing message, so they are asserted
// here rather than discovered there.
func TestLoad_NotificationDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, validYAML))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	n := cfg.Notification
	if !n.Enabled {
		t.Error("notification.enabled defaults to false; the module is a capability, not an optional middleware")
	}

	for _, tc := range []struct {
		name string
		got  any
		want any
	}{
		{"dispatcher.poll_interval", n.Dispatcher.PollInterval, 5 * time.Second},
		{"dispatcher.batch_size", n.Dispatcher.BatchSize, 50},
		{"dispatcher.workers", n.Dispatcher.Workers, 2},
		{"dispatcher.max_attempts", n.Dispatcher.MaxAttempts, 6},
		{"dispatcher.backoff_base", n.Dispatcher.BackoffBase, 30 * time.Second},
		{"dispatcher.backoff_cap", n.Dispatcher.BackoffCap, time.Hour},
		{"dispatcher.stalled_after", n.Dispatcher.StalledAfter, 5 * time.Minute},
		{"dispatcher.drain_timeout", n.Dispatcher.DrainTimeout, 30 * time.Second},
		{"email.smtp_port", n.Email.SMTPPort, 587},
		{"email.timeout", n.Email.Timeout, 10 * time.Second},
		// Empty means "authenticate as from", which is the mailbox-provider
		// case; a relay whose credential is not an address needs it set.
		{"email.username", n.Email.Username, ""},
		{"log_sender.channel", n.LogSender.Channel, "email"},
	} {
		if tc.got != tc.want {
			t.Errorf("notification.%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}

	// Both of these settle rows without sending anything, or send without
	// being configured to. Neither may arrive by omission.
	if n.Email.Enabled {
		t.Error("notification.email.enabled defaults to true")
	}
	if n.LogSender.Enabled {
		t.Error("notification.log_sender.enabled defaults to true; the log sender settles rows as sent without sending")
	}
}

// The same Viper trap db.password has: the key is absent from every YAML file,
// so AutomaticEnv alone would not surface it to Unmarshal and the module would
// refuse to start with email enabled and a credential that was set correctly.
func TestLoad_NotificationEmailPasswordEnvFallback(t *testing.T) {
	cfgPath := writeConfig(t, validYAML)

	t.Run("env set", func(t *testing.T) {
		t.Setenv("APP_NOTIFICATION_EMAIL_PASSWORD", "smtp-secret-from-env")

		cfg, err := Load(cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.Notification.Email.Password.Reveal(); got != "smtp-secret-from-env" {
			t.Errorf("password = %q, want it from the environment", got)
		}
	})

	t.Run("env unset", func(t *testing.T) {
		cfg, err := Load(cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.Notification.Email.Password.IsEmpty() {
			t.Error("a password appeared from somewhere other than the environment")
		}
	})
}

func TestLoad_NotificationEnvOverrides(t *testing.T) {
	t.Setenv("APP_NOTIFICATION_ENABLED", "false")
	t.Setenv("APP_NOTIFICATION_DISPATCHER_WORKERS", "8")
	t.Setenv("APP_NOTIFICATION_DISPATCHER_BATCH_SIZE", "5")
	t.Setenv("APP_NOTIFICATION_DISPATCHER_STALLED_AFTER", "90s")
	t.Setenv("APP_NOTIFICATION_EMAIL_ENABLED", "true")
	t.Setenv("APP_NOTIFICATION_EMAIL_SMTP_HOST", "smtp.example.test")
	t.Setenv("APP_NOTIFICATION_EMAIL_USERNAME", "AKIAEXAMPLE")
	t.Setenv("APP_NOTIFICATION_EMAIL_TIMEOUT", "45s")
	t.Setenv("APP_NOTIFICATION_LOG_SENDER_ENABLED", "true")

	cfg, err := Load(writeConfig(t, validYAML))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	n := cfg.Notification
	if n.Enabled {
		t.Error("APP_NOTIFICATION_ENABLED did not take effect")
	}
	if n.Dispatcher.Workers != 8 {
		t.Errorf("workers = %d, want 8", n.Dispatcher.Workers)
	}
	if n.Dispatcher.BatchSize != 5 {
		t.Errorf("batch_size = %d, want 5", n.Dispatcher.BatchSize)
	}
	if n.Dispatcher.StalledAfter != 90*time.Second {
		t.Errorf("stalled_after = %s, want 90s", n.Dispatcher.StalledAfter)
	}
	if !n.Email.Enabled || n.Email.SMTPHost != "smtp.example.test" {
		t.Errorf("email overrides did not take effect: %+v", n.Email)
	}
	if n.Email.Username != "AKIAEXAMPLE" || n.Email.Timeout != 45*time.Second {
		t.Errorf("email username/timeout overrides did not take effect: %+v", n.Email)
	}
	if !n.LogSender.Enabled {
		t.Error("APP_NOTIFICATION_LOG_SENDER_ENABLED did not take effect")
	}
}

// config/default.yaml carries the section, and carries it correctly: bad
// indentation in a nested block maps nothing and is invisible, because every
// key defaults to the same value the file states.
func TestDefaultYAMLShipsTheNotificationSection(t *testing.T) {
	t.Setenv("APP_DB_PASSWORD", "a-real-dev-password")

	cfg, err := Load(filepath.Join("..", "..", "config", "default.yaml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	n := cfg.Notification
	if !n.Enabled {
		t.Error("config/default.yaml disables the notification module")
	}
	if n.Dispatcher.BatchSize == 0 || n.Dispatcher.PollInterval == 0 || n.Dispatcher.DrainTimeout == 0 {
		t.Errorf("the dispatcher block did not map: %+v", n.Dispatcher)
	}
	if n.Email.SMTPPort == 0 || n.Email.Timeout == 0 {
		t.Errorf("the email block did not map: %+v", n.Email)
	}
	if n.LogSender.Channel == "" {
		t.Errorf("the log_sender block did not map: %+v", n.LogSender)
	}

	// The credential is env-only, and a YAML file that carried it would be a
	// secret in the repository.
	if !n.Email.Password.IsEmpty() {
		t.Error("config/default.yaml ships an SMTP password")
	}
	if n.Email.Enabled {
		t.Error("config/default.yaml enables SMTP delivery")
	}
	if n.LogSender.Enabled {
		t.Error("config/default.yaml enables the log sender")
	}
}
