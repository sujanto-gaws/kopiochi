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

// TestLoad_JWTSecretEnvFallback verifies the same env-binding behavior for
// the nested plugins.auth.jwt.config.secret map key, which is also
// intentionally absent from the YAML config. The plugin is left disabled
// here on purpose: this test is only about the env value reaching the
// decoded map, not about placeholder validation (that behavior — rejecting
// an enabled jwt-auth plugin with an empty/placeholder secret — is covered
// separately by TestLoad_RejectsPlaceholderSecrets).
func TestLoad_JWTSecretEnvFallback(t *testing.T) {
	cfgPath := writeConfig(t, validYAML+`
plugins:
  auth:
    jwt:
      enabled: false
      provider: jwt-auth
      config:
        expiry: "24h"
        issuer: "kopiochi"
`)

	t.Run("env set", func(t *testing.T) {
		t.Setenv("APP_JWT_SECRET", "jwt-secret-from-env")
		cfg, err := Load(cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		secret, _ := cfg.Plugins.Auth["jwt"].Config["secret"].(string)
		if secret != "jwt-secret-from-env" {
			t.Errorf("jwt secret = %q, want %q", secret, "jwt-secret-from-env")
		}
	})

	t.Run("env unset", func(t *testing.T) {
		cfg, err := Load(cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		secret, _ := cfg.Plugins.Auth["jwt"].Config["secret"].(string)
		if secret != "" {
			t.Errorf("jwt secret = %q, want empty (must not silently fall back to a default)", secret)
		}
	})
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

// TestLoad_RejectsPlaceholderJWTSecret is the same exit criterion applied to
// the legacy jwt-auth plugin secret, but only when that plugin is actually
// enabled — an unconfigured, disabled-by-default plugin entry must not fail
// validation (that's the shipped config/default.yaml state).
func TestLoad_RejectsPlaceholderJWTSecret(t *testing.T) {
	cfgPath := writeConfig(t, validYAML+`
plugins:
  auth:
    jwt:
      enabled: true
      provider: jwt-auth
      config:
        expiry: "24h"
        issuer: "kopiochi"
`)
	t.Setenv("APP_JWT_SECRET", "CHANGEME_GENERATE_YOUR_OWN_JWT_SECRET_DO_NOT_USE_THIS_VALUE")

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load succeeded with a placeholder plugins.auth.jwt.config.secret while the plugin is enabled; the server must refuse to start")
	}
	if !strings.Contains(err.Error(), "plugins.auth.jwt.config.secret") {
		t.Errorf("error = %v, want it to mention plugins.auth.jwt.config.secret", err)
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
			User:     "postgres",
			Password: "a-real-dev-password",
			Name:     "kopiochi",
			MaxConns: 10,
			MinConns: 2,
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
