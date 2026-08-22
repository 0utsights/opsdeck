# OpsDeck

OpsDeck is a small, btop-inspired terminal dashboard for personal Linux servers,
AI-agent heartbeats, and lightweight workflows. One native Go binary provides both
the TUI and remote metrics probe; there is no web server or metrics database.

## Layout

- **Overview page**: the original AI-agent, server, and workflow dashboard.
- **Fleet page**: a full-screen server and container view. OpsDeck rotates between
  pages every 20 seconds by default; `page_seconds` controls the interval.
- **AI agents**: heartbeat cards read from `~/.local/state/opsdeck/agents/*.json`.
- **Servers**: local or SSH CPU, memory, disk, network, container health, and small
  history graphs. HTTP-only services can be included as availability cards.
- **Workflows**: a persistent to-do queue. Press `a` to add an item and `space` to
  toggle completion.

The layout switches from the sketch's two-column composition to stacked panels on
smaller terminals. It follows btop's dense boxes, restrained theme, keyboard focus,
and low-overhead native collection without copying btop source.

Structural lines use console-safe square box characters, with solid usage bars and
high-contrast colors that remain readable on a physical Linux console. Color remains
24-bit when the terminal supports it.

## Controls

| Key | Action |
| --- | --- |
| `left` / `right`, `[` / `]` | Change page and reset its timer |
| `p` | Pause or resume automatic page rotation |
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
`~/.config/opsdeck/config.json`. Edit that file to define local, SSH, and HTTP
targets. Runtime state stays under `~/.local/state/opsdeck` and is never committed.

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
