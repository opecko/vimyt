package main

import (
	"fmt"
	"log"
	"os"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Sadoaz/vimyt/internal/discord"
	"github.com/Sadoaz/vimyt/internal/model"
	"github.com/Sadoaz/vimyt/internal/mpris"
	"github.com/Sadoaz/vimyt/internal/player"
	"github.com/Sadoaz/vimyt/internal/tui"
)

func main() {
	p := player.New()
	if msg := p.ErrMsg(); msg != "" {
		log.Printf("player: %s", msg)
	}

	if runtime.GOOS == "linux" {
		m, err := mpris.New(p)
		if err != nil {
			log.Printf("mpris: %v", err)
		} else {
			defer m.Close()
		}
	}

	// Discord Rich Presence. Starts disabled; the TUI enables it from the
	// persisted session settings. Safe no-op if Discord is not running.
	dc := discord.New(p, false, false, "")
	defer dc.Close()

	plStore, err := model.NewPlaylistStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading playlists: %v\n", err)
		os.Exit(1)
	}

	app := tui.New(plStore, p, dc)
	p2 := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p2.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running vimyt: %v\n", err)
		os.Exit(1)
	}
}
