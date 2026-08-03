package db

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sujanto-gaws/kopiochi/internal/config"
	"github.com/sujanto-gaws/kopiochi/internal/platform/secret"
)

func dsnConfig() config.DB {
	return config.DB{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "simple",
		Name:     "kopiochi",
		SSLMode:  "disable",
	}
}

// TestBuildDSN_EscapesSpecialCharactersInPassword is the regression test for
// Problem 1 in persistence-and-pooling.md.
//
// Verified against the fmt.Sprintf DSN this replaced: "pa/ss", "pa?ss" and
// "pa#ss" all fail with `invalid port ":pa" after host`, and "pa%ss" with
// `invalid URL escape "%ss"`. ("p@ss", the example that document used, in
// fact parses fine — pgx splits userinfo on the last @ — so it is kept here
// as coverage rather than as the regression.)
//
// The assertion goes through pgxpool.ParseConfig, the same parser the real
// connection path uses, rather than string-matching the URL: what matters is
// what the driver makes of the string, not how it looks.
func TestBuildDSN_EscapesSpecialCharactersInPassword(t *testing.T) {
	passwords := []string{
		"p@ss",
		"pa:ss",
		"pa/ss",
		"pa?ss",
		"pa#ss",
		"pa%ss",
		"p@ss:w/rd?#%",
		"~!@#$%^&*()_+{}|:<>?",
	}

	for _, pw := range passwords {
		t.Run(pw, func(t *testing.T) {
			cfg := dsnConfig()
			cfg.Password = secret.String(pw)

			parsed, err := pgxpool.ParseConfig(BuildDSN(cfg))
			if err != nil {
				t.Fatalf("ParseConfig failed for password %q: %v", pw, err)
			}

			if got := parsed.ConnConfig.Host; got != "localhost" {
				t.Errorf("host = %q, want %q (a mis-escaped password corrupts the host)", got, "localhost")
			}
			if got := parsed.ConnConfig.Port; got != 5432 {
				t.Errorf("port = %d, want 5432", got)
			}
			if got := parsed.ConnConfig.Database; got != "kopiochi" {
				t.Errorf("database = %q, want %q", got, "kopiochi")
			}
			if got := parsed.ConnConfig.User; got != "postgres" {
				t.Errorf("user = %q, want %q", got, "postgres")
			}
			if got := parsed.ConnConfig.Password; got != pw {
				t.Errorf("password round-tripped as %q, want %q", got, pw)
			}
		})
	}
}

// TestBuildDSN_BracketsIPv6Host covers the other half of Problem 1: the old
// "%s:%d" format produced "::1:5432", which is not a parseable authority.
// net.JoinHostPort brackets it.
func TestBuildDSN_BracketsIPv6Host(t *testing.T) {
	cfg := dsnConfig()
	cfg.Host = "::1"

	dsn := BuildDSN(cfg)
	if !strings.Contains(dsn, "[::1]:5432") {
		t.Errorf("DSN = %q, want it to contain a bracketed IPv6 authority [::1]:5432", dsn)
	}

	parsed, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("ParseConfig failed for an IPv6 host: %v", err)
	}
	if got := parsed.ConnConfig.Host; got != "::1" {
		t.Errorf("host = %q, want %q", got, "::1")
	}
}

// TestBuildDSN_SetsSSLModeAndApplicationName proves the query parameters
// survive escaping. application_name is what lets a DBA attribute connections
// in pg_stat_activity to this service.
func TestBuildDSN_SetsSSLModeAndApplicationName(t *testing.T) {
	cfg := dsnConfig()
	cfg.SSLMode = "require"

	parsed, err := pgxpool.ParseConfig(BuildDSN(cfg))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	if got := parsed.ConnConfig.RuntimeParams["application_name"]; got != "kopiochi" {
		t.Errorf("application_name = %q, want %q", got, "kopiochi")
	}
	if parsed.ConnConfig.TLSConfig == nil {
		t.Error("sslmode=require did not produce a TLS config")
	}
}
