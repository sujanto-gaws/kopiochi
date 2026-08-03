package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/sujanto-gaws/kopiochi/internal/config"
)

// healthcheckTimeout bounds the whole probe. A container healthcheck that can
// hang is worse than one that fails: the orchestrator waits on it instead of
// restarting the instance.
const healthcheckTimeout = 3 * time.Second

// newHealthcheckCmd builds the `healthcheck` subcommand.
//
// It exists because the runtime image is distroless: no shell, no curl, no
// wget. The conventional `CMD-SHELL curl -f localhost:8080/healthz` cannot run
// there, and the usual workaround — reverting to an image that has a shell —
// trades a real security property for a convenience. Probing from the binary
// that is already in the image costs nothing and keeps the image empty.
//
// It queries /healthz, not /readyz. Liveness asks "is this process wedged",
// and answering it with a readiness check would restart a healthy instance
// every time the database blipped — turning a brief dependency outage into a
// restart loop.
func newHealthcheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "healthcheck",
		Short: "Probe the local /healthz endpoint and exit non-zero if it fails",
		Long: "Intended for a container HEALTHCHECK. Exits 0 when the server " +
			"answers 200 on /healthz, non-zero otherwise.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, _ := cmd.Flags().GetString("config")
			return healthcheck(cmd.Context(), cfgPath)
		},
	}
	cmd.Flags().StringP("config", "c", "config/default.yaml", "Path to config file")
	return cmd
}

func healthcheck(ctx context.Context, cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Always probe loopback, whatever the server binds. A server on 0.0.0.0 is
	// reachable on 127.0.0.1, and probing the configured host would make the
	// check leave the container — measuring the network rather than the
	// process.
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Server.Port))
	url := "http://" + addr + "/healthz"

	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("probe %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("probe %s: status %d", url, resp.StatusCode)
	}
	return nil
}
