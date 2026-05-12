package version

// Version is set at build time via:
//
//	go build -ldflags "-X github.com/sujanto-gaws/kopiochi/internal/version.Version=1.2.3"
//
// Falls back to "dev" when running without ldflags (e.g. go run).
var Version = "dev"
