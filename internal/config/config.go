package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
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
}

type DB struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Name     string `mapstructure:"name"`
	SSLMode  string `mapstructure:"sslmode"`
	MaxConns int32  `mapstructure:"max_conns"`
	MinConns int32  `mapstructure:"min_conns"`
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

	// Defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.read_header_timeout", "10s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.idle_timeout", "120s")
	v.SetDefault("server.shutdown_timeout", "30s")
	v.SetDefault("server.request_timeout", "60s")
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
	return &cfg, nil
}
