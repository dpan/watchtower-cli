# watchtower-cli

Command-line client for the [Watchtower HTTP API](https://watchtower.nickfedor.com/v1.23.2/http-api/overview/) (v1.23.2).

## Install

```sh
go install github.com/dpan/watchtower-cli/cmd/watchtower-cli@latest
```

Or from a clone:

```sh
go build -o watchtower-cli ./cmd/watchtower-cli
```

You need a Watchtower instance with the HTTP API enabled.

## Usage

```sh
watchtower-cli --url http://localhost:8080 status
watchtower-cli --target homelab update --image nginx:latest
watchtower-cli health live
watchtower-cli containers details --name nginx
```

Global flags: `--url`, `--target` / `-t`, `--token`, `--events-token`, `--timeout`, `--insecure` / `-k`, `--cacert`.

With no command, the CLI prints help. URL comes from `--url`, `--target`, or `WATCHTOWER_HTTP_API_URL`.

## Configuration

Copy `targets.toml.example` to `./targets.toml` or `~/.config/watchtower-cli/targets.toml`:

```toml
[[target]]
name = "homelab"
url = "http://192.168.1.100:8080"
token = "your-api-token"
events_token = "your-events-token"
```

Override the path with `WATCHTOWER_CLI_CONFIG`. Tokens: `WATCHTOWER_HTTP_API_TOKEN` and `WATCHTOWER_HTTP_API_EVENTS_TOKEN` (`/v1/events` uses a separate token).

Do not commit `targets.toml`. Keep it mode 0600. `watchtower-cli targets` reports when that file is readable by group or others.

## Development

[mise](https://mise.jdx.dev/) pins Go and the `usage` CLI:

```sh
mise install
mise run build    # dist/watchtower-cli
mise run install  # ~/.local/bin/watchtower-cli
mise run test
mise run ci       # vet + test + build
```

The CLI surface is defined in `watchtower.usage.kdl` (completions, docs). Parsing is cobra until usage-go is stable. Regenerated completions live in `completions/` (`mise run completions`); they need the `usage` CLI on `PATH` at complete-time.

## License

MIT. Compatible with Watchtower’s Apache 2.0 license. See `LICENSE`.
