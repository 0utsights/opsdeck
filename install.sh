#!/usr/bin/env sh
set -eu

config_home=${XDG_CONFIG_HOME:-"$HOME/.config"}
state_home=${XDG_STATE_HOME:-"$HOME/.local/state"}
bin_home="$HOME/.local/bin"

go test ./...
go build -trimpath -ldflags='-s -w' -o opsdeck .

install -d -m 700 "$bin_home" "$config_home/opsdeck" "$state_home/opsdeck/agents"
install -m 755 opsdeck "$bin_home/opsdeck"

if [ ! -f "$config_home/opsdeck/config.json" ]; then
  install -m 600 config.example.json "$config_home/opsdeck/config.json"
fi
if [ ! -f "$state_home/opsdeck/workflows.json" ]; then
  install -m 600 workflows.example.json "$state_home/opsdeck/workflows.json"
fi

printf 'Installed OpsDeck at %s\n' "$bin_home/opsdeck"
printf 'Configuration: %s\n' "$config_home/opsdeck/config.json"
printf 'Ensure %s is on PATH, then run: opsdeck\n' "$bin_home"
