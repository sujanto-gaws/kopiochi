package config

import (
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server  `mapstructure:"server"`
	DB      `mapstructure:"db"`
	Log     `mapstructure:"log"`
	Plugins `mapstructure:"plugins"`
}

type Server struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
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
	v.SetDefault("db.host", "localhost")
	v.SetDefault("db.port", 5432)
	v.SetDefault("db.sslmode", "disable")
	v.SetDefault("db.max_conns", 10)
	v.SetDefault("db.min_conns", 2)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
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
