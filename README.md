# OpsDeck

OpsDeck is a small, btop-inspired terminal dashboard for personal Linux servers,
AI-agent heartbeats, and lightweight workflows. One native Go binary provides both
the TUI and remote metrics probe; there is no web server or metrics database.

## Layout

- **AI agents**: heartbeat cards read from `~/.local/state/opsdeck/agents/*.json`.
- **Servers**: local or SSH CPU, memory, disk, network, container health, sites, and
  small history graphs.
- **Sites**: configured container-name patterns place each site beneath every host
  currently running it. Moving containers moves the site on the next refresh.
- **Workflows**: a persistent to-do queue plus live host-migration states derived
  from container placement.

The layout switches from the sketch's two-column composition to stacked panels on
smaller terminals. It follows btop's dense boxes, restrained theme, keyboard focus,
and low-overhead native collection without copying btop source.

Structural lines use console-safe square box characters, with solid usage bars and
high-contrast colors that remain readable on a physical Linux console. Color remains
24-bit when the terminal supports it.

## Controls

| Key | Action |
| --- | --- |
| `tab` / `shift-tab` | Change panel |
| `j` / `k`, arrows | Move selection |
| `space` | Toggle selected workflow |
| `a` | Add a workflow |
| `q` | Quit |

## Agent heartbeat contract

An agent writes one JSON file atomically into the configured agents directory:

```json
{
  "id": "research-1",
  "name": "Research agent",
  "status": "running",
  "task": "Reviewing provider prices",
  "model": "gpt-5",
  "pid": 1234,
  "updated_at": "2026-08-22T14:00:00Z"
}
```

Heartbeats older than 90 seconds are displayed as stale. The future agent runner can
use this contract without coupling the TUI to a particular AI framework.

## Requirements

- Linux on the dashboard host and metric-probe hosts
- Go 1.24 or newer to build
- OpenSSH client for remote probes
- Docker is optional; container health appears when the current user can run
  `docker ps`

## Install on Ubuntu

```sh
git clone https://github.com/0utsights/opsdeck.git
cd opsdeck
./install.sh
opsdeck
```

The installer creates a private user configuration at
`~/.config/opsdeck/config.json`. Edit that file to define local and SSH hosts,
site health checks, container matching, and migrations. Runtime state stays under
`~/.local/state/opsdeck` and is never committed.
Site names, URLs, container patterns, SSH addresses, usernames, and migration plans
belong only in this private configuration; the repository ships generic examples.

## Rotate independent TUIs

`opsdeck-carousel` keeps independent terminal applications running in separate tmux
windows and rotates the session between them. Edit the private configuration at
`~/.config/opsdeck/carousel.conf`:

```ini
interval_seconds=20
session_name=opsdeck
window=opsdeck|opsdeck
window=system-monitor|btop
window=project-tui|your-project-command
```

Then enable the user service:

```sh
systemctl --user daemon-reload
systemctl --user enable --now opsdeck-carousel.service
```

The carousel operates above the applications, so each TUI remains an independent
process with its own keyboard handling and screen state. Add another `window=` line
to extend the rotation.

## Site placement and migration states

Each `sites` entry maps one logical site to one or more container-name fragments.
Each optional `migrations` entry names a source and target server ID. OpsDeck derives
`SOURCE ONLY`, `MIRRORED`, `MOVED`, or `MISSING` directly from live Docker probes;
there is no second placement database to keep synchronized.

## Remote probes

Install the same binary on each remote Linux server:

```sh
sudo install -m 755 ~/.local/bin/opsdeck /usr/local/bin/opsdeck
/usr/local/bin/opsdeck --probe
```

Use a dedicated SSH key restricted to the probe command. A suitable
`authorized_keys` prefix is:

```text
restrict,command="sudo -n /usr/local/bin/opsdeck --probe"
```

OpsDeck invokes probes on demand and does not leave an agent daemon running.

## Development

```sh
go test ./...
go build -trimpath -ldflags="-s -w" -o opsdeck .
./opsdeck --config ./config.example.json --snapshot 140x42
```
