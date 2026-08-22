package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"
)

func main() {
	probe := flag.Bool("probe", false, "print local server metrics as JSON")
	configPath := flag.String("config", "", "path to config.json")
	snapshot := flag.String("snapshot", "", "render one frame as WIDTHxHEIGHT text")
	flag.Parse()

	if *probe {
		if err := printProbe(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	app := newApp(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app.refresh(ctx)

	if *snapshot != "" {
		parts := strings.Split(strings.ToLower(*snapshot), "x")
		if len(parts) != 2 {
			fmt.Fprintln(os.Stderr, "snapshot must be WIDTHxHEIGHT")
			os.Exit(2)
		}
		w, _ := strconv.Atoi(parts[0])
		h, _ := strconv.Atoi(parts[1])
		if err := app.snapshot(w, h); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	screen, err := tcell.NewScreen()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := screen.Init(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer screen.Fini()
	screen.EnableMouse()

	events := make(chan tcell.Event, 16)
	go func() {
		for {
			events <- screen.PollEvent()
		}
	}()
	ticker := time.NewTicker(time.Duration(cfg.RefreshSeconds) * time.Second)
	defer ticker.Stop()
	app.draw(screen)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			app.refresh(ctx)
			app.draw(screen)
		case ev := <-events:
			switch e := ev.(type) {
			case *tcell.EventResize:
				screen.Sync()
			case *tcell.EventKey:
				if app.handleKey(e) {
					return
				}
			}
			app.draw(screen)
		}
	}
}
