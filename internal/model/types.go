// Package model defines the data types and persistence for vimyt.
package model

import "time"

// Track represents a single music track.
type Track struct {
	ID           string
	Title        string
	Artist       string
	Duration     time.Duration
	StreamURL    string
	ThumbnailURL string
}

// PlayerState represents the current playback state.
type PlayerState int

const (
	Stopped PlayerState = iota
	Playing
	Paused
)

// PlayerStatus holds the current player state for the TUI to display.
type PlayerStatus struct {
	State    PlayerState
	Track    *Track
	Position time.Duration
	Volume   int // 0-100
}

type PlayerInterface interface {
	Status() PlayerStatus
	Play(t *Track)
	Pause()
	Stop()
	Seek(deltaSec float64)
	SeekAbsolute(sec float64)
	SetVolume(v int)

	// Shuffle state (set by TUI, queried by MPRIS)
	SetShuffle(bool)
	IsShuffle() bool

	// Loop state (set by TUI, queried by MPRIS)
	SetLoopTrack(bool)
	IsLoopTrack() bool
	SetLoopPlaylist(bool)
	IsLoopPlaylist() bool

	// MPRIS action channels
	OnNext() chan struct{}
	OnPrev() chan struct{}
	OnSeek() chan int64
	OnSetPosition() chan int64
}
