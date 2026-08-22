#!/usr/bin/env sh
set -eu

config_home=${XDG_CONFIG_HOME:-"$HOME/.config"}
state_home=${XDG_STATE_HOME:-"$HOME/.local/state"}
bin_home="$HOME/.local/bin"

go test ./...
go build -trimpath -ldflags='-s -w' -o opsdeck .

install -d -m 700 "$bin_home" "$config_home/opsdeck" "$state_home/opsdeck/agents"
install -m 755 opsdeck "$bin_home/opsdeck"
install -m 755 opsdeck-carousel "$bin_home/opsdeck-carousel"

if [ ! -f "$config_home/opsdeck/config.json" ]; then
  install -m 600 config.example.json "$config_home/opsdeck/config.json"
fi
if [ ! -f "$config_home/opsdeck/carousel.conf" ]; then
  install -m 600 carousel.example.conf "$config_home/opsdeck/carousel.conf"
fi
install -d -m 700 "$config_home/systemd/user"
install -m 644 opsdeck-carousel.service "$config_home/systemd/user/opsdeck-carousel.service"
if [ ! -f "$state_home/opsdeck/workflows.json" ]; then
  install -m 600 workflows.example.json "$state_home/opsdeck/workflows.json"
fi

printf 'Installed OpsDeck at %s\n' "$bin_home/opsdeck"
printf 'Configuration: %s\n' "$config_home/opsdeck/config.json"
printf 'TUI carousel: %s\n' "$config_home/opsdeck/carousel.conf"
printf 'Ensure %s is on PATH, then run: opsdeck\n' "$bin_home"
