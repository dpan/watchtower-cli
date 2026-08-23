package cli

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

func (o *Options) addCommands(root *cobra.Command) {
	root.AddCommand(
		o.updateCmd(),
		o.checkCmd(),
		o.containersCmd(),
		o.historyCmd(),
		o.imagesCmd(),
		o.configCmd(),
		o.statusCmd(),
		o.metricsCmd(),
		o.eventsCmd(),
		o.probeCmd(),
		o.healthCmd(),
	)
}

func (o *Options) updateCmd() *cobra.Command {
	var images, containers []string
	var async bool
	var apiTimeout string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Trigger container updates (POST /v1/update)",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := filterQuery(images, containers)
			if async {
				q.Set("async", "true")
			}
			if apiTimeout != "" {
				q.Set("timeout", apiTimeout)
			}
			return o.call(cmd, http.MethodPost, "/v1/update", q)
		},
	}
	cmd.Flags().StringArrayVarP(&images, "image", "i", nil, "image filter (repeatable)")
	cmd.Flags().StringArrayVarP(&containers, "container", "c", nil, "container name or regex filter (repeatable)")
	cmd.Flags().BoolVar(&async, "async", false, "run the update in the background (HTTP 202)")
	cmd.Flags().StringVar(&apiTimeout, "api-timeout", "", "Watchtower handler timeout override (e.g. 2m)")
	return cmd
}

func (o *Options) checkCmd() *cobra.Command {
	var images, containers []string
	var apiTimeout string
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check for available updates without applying them (POST /v1/check)",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := filterQuery(images, containers)
			if apiTimeout != "" {
				q.Set("timeout", apiTimeout)
			}
			return o.call(cmd, http.MethodPost, "/v1/check", q)
		},
	}
	cmd.Flags().StringArrayVarP(&images, "image", "i", nil, "image filter (repeatable)")
	cmd.Flags().StringArrayVarP(&containers, "container", "c", nil, "container name or regex filter (repeatable)")
	cmd.Flags().StringVar(&apiTimeout, "api-timeout", "", "Watchtower handler timeout override (e.g. 2m)")
	return cmd
}

func (o *Options) containersCmd() *cobra.Command {
	var names, images []string
	cmd := &cobra.Command{
		Use:   "containers",
		Short: "List watched containers (GET /v1/containers)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.call(cmd, http.MethodGet, "/v1/containers", nameQuery(names, images))
		},
	}
	cmd.Flags().StringArrayVarP(&names, "name", "n", nil, "exact container name filter (repeatable)")
	cmd.Flags().StringArrayVarP(&images, "image", "i", nil, "image filter (repeatable)")

	var dNames, dImages []string
	details := &cobra.Command{
		Use:   "details",
		Short: "Detailed container status (GET /v1/containers/details)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.call(cmd, http.MethodGet, "/v1/containers/details", nameQuery(dNames, dImages))
		},
	}
	details.Flags().StringArrayVarP(&dNames, "name", "n", nil, "exact container name filter (repeatable)")
	details.Flags().StringArrayVarP(&dImages, "image", "i", nil, "image filter (repeatable)")
	cmd.AddCommand(details)
	return cmd
}

func (o *Options) historyCmd() *cobra.Command {
	var since, until string
	var limit int
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Scan history from the in-memory ring buffer (GET /v1/history)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.call(cmd, http.MethodGet, "/v1/history", historyQuery(since, until, limit))
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "RFC3339 timestamp; entries at or after")
	cmd.Flags().StringVar(&until, "until", "", "RFC3339 timestamp; entries at or before")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of entries")
	return cmd
}

func (o *Options) imagesCmd() *cobra.Command {
	var names, ids []string
	cmd := &cobra.Command{
		Use:   "images",
		Short: "List tracked images (GET /v1/images)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.call(cmd, http.MethodGet, "/v1/images", imageQuery(names, ids))
		},
	}
	cmd.Flags().StringArrayVarP(&names, "name", "n", nil, "exact image name filter (repeatable)")
	cmd.Flags().StringArrayVar(&ids, "id", nil, "image ID filter (repeatable)")
	return cmd
}

func (o *Options) configCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show the active Watchtower configuration (GET /v1/config)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.call(cmd, http.MethodGet, "/v1/config", nil)
		},
	}
}

func (o *Options) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Summary of the most recent scan (GET /v1/status)",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := o.Client.Do(cmd.Context(), http.MethodGet, "/v1/status", nil)
			if err != nil {
				return err
			}
			if res.Status >= 300 {
				return httpError(res)
			}
			if res.Status == http.StatusNoContent {
				fmt.Fprintln(cmd.OutOrStdout(), "204 No Content: no update scan has run yet; run: watchtower-cli update")
				return nil
			}
			return writeBody(cmd.OutOrStdout(), res.Body)
		},
	}
}

func (o *Options) metricsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "metrics",
		Short: "Prometheus metrics (GET /v1/metrics)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.call(cmd, http.MethodGet, "/v1/metrics", nil)
		},
	}
}

func (o *Options) eventsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "events",
		Short: "Stream operational events via SSE (GET /v1/events)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if o.EventsToken == "" {
				return fmt.Errorf("events token required (--events-token or WATCHTOWER_HTTP_API_EVENTS_TOKEN)")
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			c := o.Client.WithToken(o.EventsToken)
			status, body, err := c.Stream(ctx, http.MethodGet, "/v1/events")
			if err != nil {
				return err
			}
			defer body.Close()
			if status >= 300 {
				b, _ := io.ReadAll(body)
				return fmt.Errorf("unexpected status %d: %s", status, strings.TrimSpace(string(b)))
			}
			scanner := bufio.NewScanner(body)
			out := cmd.OutOrStdout()
			for scanner.Scan() {
				fmt.Fprintln(out, scanner.Text())
			}
			return scanner.Err()
		},
	}
}

func (o *Options) probeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "probe",
		Short: "TCP connectivity check against the API URL (does not send a token)",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := o.Client.Probe(cmd.Context(), o.Timeout)
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), report)
		},
	}
}

func (o *Options) healthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Watchtower health probes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("health subcommand required: live, ready, or startup")
		},
	}
	cmd.AddCommand(
		o.healthProbeCmd("live", "Liveness probe (GET /livez)", "/livez", "liveness"),
		o.healthProbeCmd("ready", "Readiness probe (GET /readyz)", "/readyz", "readiness"),
		o.healthProbeCmd("startup", "Startup probe (GET /startupz)", "/startupz"),
	)
	return cmd
}

func (o *Options) healthProbeCmd(use, short, path string, aliases ...string) *cobra.Command {
	return &cobra.Command{
		Use:     use,
		Short:   short,
		Aliases: aliases,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := o.Client.Do(cmd.Context(), http.MethodGet, path, nil)
			if err != nil {
				return err
			}
			if res.Status >= 300 {
				return fmt.Errorf("%s: %s (%d)", path, strings.TrimSpace(string(res.Body)), res.Status)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %d OK\n", path, res.Status)
			return nil
		},
	}
}

func (o *Options) call(cmd *cobra.Command, method, path string, q url.Values) error {
	res, err := o.Client.Do(cmd.Context(), method, path, q)
	if err != nil {
		return err
	}
	if res.Status >= 300 {
		return httpError(res)
	}
	if res.Status == http.StatusNoContent {
		fmt.Fprintln(cmd.OutOrStdout(), "204 No Content")
		return nil
	}
	return writeBody(cmd.OutOrStdout(), res.Body)
}
