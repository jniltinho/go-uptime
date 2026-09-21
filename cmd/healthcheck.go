package cmd

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/spf13/cobra"
)

const (
	urlFlagName = "url"

	// healthcheckTimeout is the total time of the check, connection and answer included
	healthcheckTimeout = 5 * time.Second
)

var errUnhealthy = errors.New("unhealthy")

// healthcheckCmd exists because the image has no shell, no curl and no wget: the binary checks itself. It reads only
// the web section of the configuration, and nothing of what the server starts: no delay, no storage and no OIDC.
var healthcheckCmd = &cobra.Command{
	Use:   "healthcheck",
	Short: "Check the /health of the local server, for the HEALTHCHECK of the image",
	Long: `Requests /health and exits with 0 when the answer is 200 and with 1 otherwise, in at most 5 seconds.

Without --url, the address, the port and the scheme come from the web section of the configuration: a wildcard address
becomes the loopback, and with web.tls the check speaks HTTPS to the loopback without verifying the certificate, which
may be self-signed. With --url the configuration is not read.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		// One deadline for the whole command, the read of the configuration included
		ctx, cancel := context.WithTimeout(cmd.Context(), healthcheckTimeout)
		defer cancel()
		target, _ := cmd.Flags().GetString(urlFlagName)
		if len(target) == 0 {
			configPath, err := resolveConfigPath(cmd)
			if err != nil {
				return err
			}
			if target, err = healthcheckTargetOf(ctx, configPath); err != nil {
				return err
			}
		}
		if err := checkHealth(ctx, target); err != nil {
			return fmt.Errorf("%s: %w", target, err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "healthy")
		return nil
	},
}

func init() {
	healthcheckCmd.Flags().String(urlFlagName, "", "URL to check instead of the /health of the configuration")
	rootCmd.AddCommand(healthcheckCmd)
}

// healthcheckTargetOf reads the web section of the configuration within the deadline of the command: a path that blocks,
// like a pipe nobody writes to, must not hold the check beyond it
func healthcheckTargetOf(ctx context.Context, configPath string) (string, error) {
	type loaded struct {
		target string
		err    error
	}
	done := make(chan loaded, 1)
	go func() {
		webConfig, err := config.LoadWebConfiguration(configPath)
		if err != nil {
			done <- loaded{err: err}
			return
		}
		done <- loaded{target: healthcheckURL(webConfig.Address, webConfig.Port, webConfig.HasTLS())}
	}()
	select {
	case result := <-done:
		return result.target, result.err
	case <-ctx.Done():
		return "", fmt.Errorf("reading the configuration: %w", ctx.Err())
	}
}

// healthcheckURL returns the URL of /health for the address the server binds to. A wildcard address is where the
// server listens, not where it can be reached: it becomes the loopback of the same family, because a server bound to
// "::" with bindv6only does not answer on 127.0.0.1. The address may come with the brackets the server accepts
// ("[::1]"), which net.JoinHostPort would double.
func healthcheckURL(address string, port int, hasTLS bool) string {
	host := strings.TrimSuffix(strings.TrimPrefix(address, "["), "]")
	if ip := net.ParseIP(host); len(host) == 0 {
		host = "127.0.0.1"
	} else if ip != nil && ip.IsUnspecified() {
		host = "127.0.0.1"
		if ip.To4() == nil {
			host = "::1"
		}
	}
	scheme := "http"
	if hasTLS {
		scheme = "https"
	}
	return scheme + "://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/health"
}

// isLoopbackTarget returns whether the URL points to this machine. Only then is the certificate not verified: the
// server of the configuration may use a self-signed certificate, or one issued for another name than the loopback, but
// a --url to another host gets the normal verification.
func isLoopbackTarget(target string) bool {
	parsed, err := url.Parse(target)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func checkHealth(ctx context.Context, target string) error {
	client := &http.Client{
		Transport: &http.Transport{
			// The proxy of the environment is for the checks of Go Uptime, never for the server checking itself
			Proxy:           nil,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: isLoopbackTarget(target)}, //nolint:gosec // only for the loopback
		},
		// A redirect is not a healthy answer
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", errUnhealthy, response.StatusCode)
	}
	return nil
}
