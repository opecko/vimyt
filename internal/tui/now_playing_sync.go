package tui

import (
	"github.com/Sadoaz/vimyt/internal/model"
)

func (a *App) publishLocalClaim(status model.PlayerStatus) {
	if a.deviceID == "" {
		return
	}

	var trackID string
	var playing bool
	if status.Track != nil {
		trackID = status.Track.ID
		playing = status.State == model.Playing
	}

	if a.suppressNextClaimPublish {
		a.suppressNextClaimPublish = false
		a.lastClaimTrackID = trackID
		a.lastClaimPlaying = playing
		return
	}

	if trackID == a.lastClaimTrackID && playing == a.lastClaimPlaying {
		return
	}

	np := model.NowPlaying{
		DeviceID:   a.deviceID,
		DeviceName: a.deviceName,
		TrackID:    trackID,
		PositionMs: status.Position.Milliseconds(),
		Playing:    playing,
	}
	if status.Track != nil {
		np.Title = status.Track.Title
		np.Artist = status.Track.Artist
	}

	if err := model.SaveNowPlaying(np); err == nil {
		a.lastClaimTrackID = trackID
		a.lastClaimPlaying = playing
	}
}

func (a *App) honourForeignClaim() {
	if a.deviceID == "" {
		return
	}
	np := model.LoadNowPlaying()
	if np == nil {
		a.foreignClaim = nil
		return
	}
	if np.DeviceID == "" || np.DeviceID == a.deviceID {
		a.foreignClaim = nil
		return
	}

	if np.Playing {
		a.foreignClaim = np
	} else {
		a.foreignClaim = nil
	}

	if !np.Playing {
		return
	}
	if np.DeviceID == a.lastSeenForeignID && np.StartedAt == a.lastSeenForeignAt {
		return
	}
	a.lastSeenForeignID = np.DeviceID
	a.lastSeenForeignAt = np.StartedAt

	status := a.player.Status()
	if status.State == model.Playing {
		a.player.Pause()
		a.suppressNextClaimPublish = true
	}
}
