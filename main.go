package main

import (
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"

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

	m, err := mpris.New(p)
	if err != nil {
		log.Printf("mpris: %v", err)
	} else {
		defer m.Close()
	}

	plStore, err := model.NewPlaylistStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading playlists: %v\n", err)
		os.Exit(1)
	}

	app := tui.New(plStore, p)
	p2 := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p2.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running vimyt: %v\n", err)
		os.Exit(1)
	}
}
