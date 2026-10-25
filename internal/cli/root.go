package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/dpan/watchtower-cli/internal/client"
	"github.com/dpan/watchtower-cli/internal/config"
)

const version = "0.2.0"

type Options struct {
	URL         string
	TargetName  string
	Token       string
	EventsToken string
	Timeout     time.Duration
	Insecure    bool
	CACert      string
	Client      *client.Client
}

func NewRoot() *cobra.Command {
	o := &Options{}
	root := &cobra.Command{
		Use:           "watchtower-cli",
		Short:         "Command-line client for the Watchtower HTTP API (v1.23.2)",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// No subcommand: RunE prints help and does not call the API.
			if skipClientSetup(cmd) || cmd.Parent() == nil {
				return nil
			}
			if err := o.resolve(cmd); err != nil {
				return err
			}
			if o.Insecure && o.CACert != "" {
				return fmt.Errorf("--cacert cannot be used with --insecure")
			}
			c := client.NewClient(o.URL, o.Token, o.Timeout, o.Insecure)
			if o.CACert != "" {
				pool, err := client.LoadRootCAs(o.CACert)
				if err != nil {
					return err
				}
				c = c.WithRootCAs(pool)
			}
			o.Client = c
			return nil
		},
	}
	root.SetVersionTemplate("watchtower-cli {{.Version}}\n")
	root.CompletionOptions.DisableDefaultCmd = true

	f := root.PersistentFlags()
	f.StringVar(&o.URL, "url", "", "Watchtower HTTP API base URL (env WATCHTOWER_HTTP_API_URL)")
	f.StringVarP(&o.TargetName, "target", "t", "", "named target from targets.toml")
	f.StringVar(&o.Token, "token", "", "API token, sent as Authorization: Bearer (env WATCHTOWER_HTTP_API_TOKEN)")
	f.StringVar(&o.EventsToken, "events-token", "", "events SSE token (env WATCHTOWER_HTTP_API_EVENTS_TOKEN)")
	f.DurationVar(&o.Timeout, "timeout", 10*time.Minute, "HTTP client timeout")
	f.BoolVarP(&o.Insecure, "insecure", "k", false, "skip TLS certificate verification")
	f.StringVar(&o.CACert, "cacert", "", "PEM CA bundle to trust for the API URL, instead of the system trust store")

	o.addCommands(root)
	return root
}

func Execute(args []string) error {
	root := NewRoot()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return err
	}
	return nil
}

func (o *Options) resolve(cmd *cobra.Command) error {
	var tgt *config.Target
	if o.TargetName != "" {
		path, err := config.ConfigPath()
		if err != nil {
			return err
		}
		if path == "" {
			return fmt.Errorf("no config file found (looked at ./%s and $XDG_CONFIG_HOME/watchtower-cli/%s)", config.ConfigFileName, config.ConfigFileName)
		}
		cfg, err := config.LoadConfig(path)
		if err != nil {
			return err
		}
		tgt, err = cfg.Target(o.TargetName)
		if err != nil {
			return err
		}
	}

	if o.URL == "" && tgt != nil {
		o.URL = tgt.URL
	}
	if o.URL == "" {
		o.URL = envOr("WATCHTOWER_HTTP_API_URL", "")
	}
	if o.URL == "" {
		return fmt.Errorf("URL required: --url, --target, or WATCHTOWER_HTTP_API_URL")
	}

	if !flagChanged(cmd, "token") {
		if tgt != nil && tgt.Token != "" {
			o.Token = tgt.Token
		} else {
			o.Token = envOr("WATCHTOWER_HTTP_API_TOKEN", "")
		}
	}
	if !flagChanged(cmd, "events-token") {
		if tgt != nil && tgt.EventsToken != "" {
			o.EventsToken = tgt.EventsToken
		} else {
			o.EventsToken = envOr("WATCHTOWER_HTTP_API_EVENTS_TOKEN", "")
		}
	}
	return nil
}

func skipClientSetup(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "help", "targets", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
			return true
		}
	}
	return false
}

func flagChanged(cmd *cobra.Command, name string) bool {
	f := cmd.Flags().Lookup(name)
	return f != nil && f.Changed
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
