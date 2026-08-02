package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad_DBPasswordEnvFallback guards against a Viper trap: AutomaticEnv()
// alone does not surface an env var to Unmarshal() unless the key is already
// known to Viper (via config file, SetDefault, or BindEnv). Since
// db.password is intentionally absent from the YAML config, it must be
// reachable only via an explicit BindEnv + APP_DB_PASSWORD.
func TestLoad_DBPasswordEnvFallback(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test.yaml")
	yamlNoPassword := `
db:
  host: "localhost"
  port: 5432
  user: "postgres"
  name: "kopiochi"
  sslmode: "disable"
`
	if err := os.WriteFile(cfgPath, []byte(yamlNoPassword), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("env set", func(t *testing.T) {
		t.Setenv("APP_DB_PASSWORD", "secret-from-env")
		cfg, err := Load(cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.DB.Password != "secret-from-env" {
			t.Errorf("cfg.DB.Password = %q, want %q", cfg.DB.Password, "secret-from-env")
		}
	})

	t.Run("env unset", func(t *testing.T) {
		cfg, err := Load(cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.DB.Password != "" {
			t.Errorf("cfg.DB.Password = %q, want empty (must not silently fall back to a default)", cfg.DB.Password)
		}
	})
}

// TestLoad_EnvOverridesFileValue extends the fallback coverage above by
// proving APP_DB_PASSWORD wins even when the config file already sets
// db.password — the tests above only ever exercise the "key absent from
// file" case (env as the only source), never "env overrides an existing
// file value", which is the actual scenario the remediation plan's
// TestEnvOverridesFile sketch (testing-strategy.md:107-113) describes.
//
// Note: that sketch asserts on cfg.DB.Password.Reveal(), which assumes a
// secret.String wrapper type slated for Phase 2.9 that does not exist yet.
// DB.Password is a plain string today, so this asserts on the string
// directly instead of inventing that type early.
func TestLoad_EnvOverridesFileValue(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test.yaml")
	yamlWithPassword := `
db:
  host: "localhost"
  port: 5432
  user: "postgres"
  password: "from-file"
  name: "kopiochi"
  sslmode: "disable"
`
	if err := os.WriteFile(cfgPath, []byte(yamlWithPassword), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("APP_DB_PASSWORD", "from-env")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Password != "from-env" {
		t.Errorf("cfg.DB.Password = %q, want %q (env must override a value already present in the file)", cfg.DB.Password, "from-env")
	}
}

// TestLoad_JWTSecretEnvFallback verifies the same env-binding behavior for
// the nested plugins.auth.jwt.config.secret map key, which is also
// intentionally absent from the YAML config.
func TestLoad_JWTSecretEnvFallback(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test.yaml")
	yamlNoSecret := `
plugins:
  auth:
    jwt:
      enabled: true
      provider: jwt-auth
      config:
        expiry: "24h"
        issuer: "kopiochi"
`
	if err := os.WriteFile(cfgPath, []byte(yamlNoSecret), 0644); err != nil {
		t.Fatal(err)
	}

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
