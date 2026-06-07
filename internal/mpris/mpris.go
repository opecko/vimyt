package mpris

import (
	"context"
	"fmt"
	"time"

	"github.com/Sadoaz/vimyt/internal/model"
	"github.com/godbus/dbus/v5"
)

const (
	dbusName    = "org.mpris.MediaPlayer2.vimyt"
	dbusPath    = "/org/mpris/MediaPlayer2"
	ifacePlayer = "org.mpris.MediaPlayer2.Player"
	ifaceRoot   = "org.mpris.MediaPlayer2"
)

type Server struct {
	player   model.PlayerInterface
	conn     *dbus.Conn
	quit     context.CancelFunc
	quitDone chan struct{}
}

func New(player model.PlayerInterface) (*Server, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		player:   player,
		conn:     conn,
		quit:     cancel,
		quitDone: make(chan struct{}),
	}

	if err := s.export(); err != nil {
		conn.Close()
		return nil, err
	}

	reply, err := conn.RequestName(dbusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		conn.Close()
		return nil, fmt.Errorf("mpris name unavailable: %s", reply)
	}

	go s.watchPlayback(ctx)

	return s, nil
}

func (s *Server) export() error {
	if err := s.conn.Export(s, dbusPath, ifaceRoot); err != nil {
		return err
	}
	if err := s.conn.Export(s, dbusPath, ifacePlayer); err != nil {
		return err
	}

	// Properties interface via ExportMethodTable (requires *dbus.Error return)
	props := map[string]any{
		"GetAll": s.getAllHandler,
		"Get":    s.getHandler,
		"Set":    s.setHandler,
	}
	return s.conn.ExportMethodTable(props, dbusPath, "org.freedesktop.DBus.Properties")
}

func (s *Server) Close() {
	s.quit()
	<-s.quitDone
	if s.conn != nil {
		s.conn.ReleaseName(dbusName)
		s.conn.Close()
	}
}

// --- org.mpris.MediaPlayer2.Root ---

func (s *Server) Raise() *dbus.Error { return nil }

func (s *Server) Quit() *dbus.Error {
	s.quit()
	return nil
}

// --- org.mpris.MediaPlayer2.Player ---

func (s *Server) Next() *dbus.Error {
	select {
	case s.player.OnNext() <- struct{}{}:
	default:
	}
	return nil
}

func (s *Server) Previous() *dbus.Error {
	select {
	case s.player.OnPrev() <- struct{}{}:
	default:
	}
	return nil
}

func (s *Server) Pause() *dbus.Error {
	if s.player.Status().State == model.Playing {
		s.player.Pause()
	}
	return nil
}

func (s *Server) Stop() *dbus.Error {
	s.player.Stop()
	return nil
}

func (s *Server) Play() *dbus.Error {
	st := s.player.Status()
	if st.Track != nil {
		s.player.Play(st.Track)
	}
	return nil
}

func (s *Server) PlayPause() *dbus.Error {
	s.player.Pause()
	return nil
}

func (s *Server) Seek(offset int64) *dbus.Error {
	select {
	case s.player.OnSeek() <- offset:
	default:
	}
	return nil
}
func (s *Server) SetPosition(o dbus.ObjectPath, pos int64) *dbus.Error {
	select {
	case s.player.OnSetPosition() <- pos:
	default:
	}
	return nil
}
func (s *Server) OpenUri(string) *dbus.Error { return nil }

// --- Property getters (return (value, *dbus.Error)) ---

func (s *Server) GetPlaybackStatus() (string, *dbus.Error) {
	st := s.player.Status().State
	switch st {
	case model.Playing:
		return "Playing", nil
	case model.Paused:
		return "Paused", nil
	default:
		return "Stopped", nil
	}
}

func (s *Server) GetLoopStatus() (string, *dbus.Error) {
	if p, ok := s.player.(interface{ IsLoopPlaylist() bool }); ok && p.IsLoopPlaylist() {
		return "Playlist", nil
	}
	if p, ok := s.player.(interface{ IsLoopTrack() bool }); ok && p.IsLoopTrack() {
		return "Track", nil
	}
	return "None", nil
}

func (s *Server) GetRate() (float64, *dbus.Error) { return 1.0, nil }

func (s *Server) GetShuffle() (bool, *dbus.Error) {
	if p, ok := s.player.(interface{ IsShuffle() bool }); ok {
		return p.IsShuffle(), nil
	}
	return false, nil
}
func (s *Server) GetMinimumRate() (float64, *dbus.Error) { return 1.0, nil }
func (s *Server) GetMaximumRate() (float64, *dbus.Error) { return 1.0, nil }
func (s *Server) GetCanPlay() (bool, *dbus.Error)        { return true, nil }
func (s *Server) GetCanPause() (bool, *dbus.Error)       { return true, nil }
func (s *Server) GetCanSeek() (bool, *dbus.Error)        { return true, nil }
func (s *Server) GetCanGoNext() (bool, *dbus.Error)      { return true, nil }
func (s *Server) GetCanGoPrevious() (bool, *dbus.Error)  { return true, nil }
func (s *Server) GetCanControl() (bool, *dbus.Error)     { return true, nil }

func (s *Server) GetVolume() (float64, *dbus.Error) {
	return float64(s.player.Status().Volume) / 100.0, nil
}

func (s *Server) GetPosition() (int64, *dbus.Error) {
	// time.Duration is nanoseconds, convert to microseconds
	return int64(s.player.Status().Position) / 1000, nil
}

func artURL(t *model.Track) string {
	if t.ThumbnailURL != "" {
		return t.ThumbnailURL
	}
	if t.ID != "" {
		return fmt.Sprintf("https://img.youtube.com/vi/%s/hqdefault.jpg", t.ID)
	}
	return ""
}

func (s *Server) GetMetadata() (map[string]dbus.Variant, *dbus.Error) {
	st := s.player.Status()
	if st.Track == nil {
		return nil, nil
	}
	t := st.Track
	m := map[string]dbus.Variant{
		"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath("/org/mpris/MediaPlayer2/TrackList/NoTrack")),
		"xesam:title":   dbus.MakeVariant(t.Title),
		"xesam:artist":  dbus.MakeVariant([]string{t.Artist}),
		"mpris:artUrl":  dbus.MakeVariant(artURL(t)),
	}
	if t.Duration > 0 {
		m["mpris:length"] = dbus.MakeVariant(uint64(t.Duration) / 1000)
	}
	return m, nil
}

// --- Property setters (return *dbus.Error) ---

func (s *Server) SetVolume(v float64) *dbus.Error {
	s.player.SetVolume(int(v * 100))
	return nil
}

func (s *Server) SetLoopStatus(loop string) *dbus.Error {
	setTrack := func(on bool) {
		if p, ok := s.player.(interface{ SetLoopTrack(bool) }); ok {
			p.SetLoopTrack(on)
		}
	}
	setPlaylist := func(on bool) {
		if p, ok := s.player.(interface{ SetLoopPlaylist(bool) }); ok {
			p.SetLoopPlaylist(on)
		}
	}
	switch loop {
	case "Track":
		setPlaylist(false)
		setTrack(true)
	case "Playlist":
		setTrack(false)
		setPlaylist(true)
	default:
		setPlaylist(false)
		setTrack(false)
	}
	return nil
}

func (s *Server) SetRate(float64) *dbus.Error { return nil }

func (s *Server) SetShuffle(on bool) *dbus.Error {
	if p, ok := s.player.(interface{ SetShuffle(bool) }); ok {
		p.SetShuffle(on)
	}
	return nil
}

// --- org.freedesktop.DBus.Properties handlers ---

func (s *Server) getAllHandler(iface string) (map[string]dbus.Variant, *dbus.Error) {
	st := s.player.Status()

	var status string
	switch st.State {
	case model.Playing:
		status = "Playing"
	case model.Paused:
		status = "Paused"
	default:
		status = "Stopped"
	}

	loopStatus, _ := s.GetLoopStatus()
	shuffle := false
	if p, ok := s.player.(interface{ IsShuffle() bool }); ok {
		shuffle = p.IsShuffle()
	}

	props := map[string]dbus.Variant{
		"PlaybackStatus": dbus.MakeVariant(status),
		"LoopStatus":     dbus.MakeVariant(loopStatus),
		"Rate":           dbus.MakeVariant(1.0),
		"Shuffle":        dbus.MakeVariant(shuffle),
		"Volume":         dbus.MakeVariant(float64(st.Volume) / 100.0),
		"MinimumRate":    dbus.MakeVariant(1.0),
		"MaximumRate":    dbus.MakeVariant(1.0),
		"CanPlay":        dbus.MakeVariant(true),
		"CanPause":       dbus.MakeVariant(true),
		"CanSeek":        dbus.MakeVariant(true),
		"CanGoNext":      dbus.MakeVariant(true),
		"CanGoPrevious":  dbus.MakeVariant(true),
		"CanControl":     dbus.MakeVariant(true),
		"Position":       dbus.MakeVariant(int64(st.Position) / 1000),
	}

	if st.Track != nil {
		t := st.Track
		meta := map[string]dbus.Variant{
			"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath("/org/mpris/MediaPlayer2/TrackList/NoTrack")),
			"xesam:title":   dbus.MakeVariant(t.Title),
			"xesam:artist":  dbus.MakeVariant([]string{t.Artist}),
			"mpris:artUrl":  dbus.MakeVariant(artURL(t)),
		}
		if t.Duration > 0 {
			meta["mpris:length"] = dbus.MakeVariant(uint64(t.Duration) / 1000)
		}
		props["Metadata"] = dbus.MakeVariant(meta)
	}

	return props, nil
}

func (s *Server) getHandler(iface, prop string) (dbus.Variant, *dbus.Error) {
	switch prop {
	case "PlaybackStatus":
		status, _ := s.GetPlaybackStatus()
		return dbus.MakeVariant(status), nil
	case "LoopStatus":
		loop, _ := s.GetLoopStatus()
		return dbus.MakeVariant(loop), nil
	case "Shuffle":
		shuffle, _ := s.GetShuffle()
		return dbus.MakeVariant(shuffle), nil
	case "Volume":
		return dbus.MakeVariant(float64(s.player.Status().Volume) / 100.0), nil
	case "Metadata":
		meta, _ := s.GetMetadata()
		return dbus.MakeVariant(meta), nil
	case "Position":
		pos, _ := s.GetPosition()
		return dbus.MakeVariant(pos), nil
	case "CanSeek":
		return dbus.MakeVariant(true), nil
	default:
		return dbus.MakeVariant(""), nil
	}
}

func (s *Server) setHandler(iface, prop string, val dbus.Variant) *dbus.Error {
	v := val.Value()
	switch prop {
	case "Volume":
		if f, ok := v.(float64); ok {
			s.player.SetVolume(int(f * 100))
		}
	case "LoopStatus":
		if str, ok := v.(string); ok {
			s.SetLoopStatus(str)
		}
	case "Shuffle":
		if b, ok := v.(bool); ok {
			s.SetShuffle(b)
		}
	}
	return nil
}

// --- Signal emitter ---

func (s *Server) emitPropertyChanged(props map[string]dbus.Variant) {
	s.conn.Emit(dbusPath,
		"org.freedesktop.DBus.Properties.PropertiesChanged",
		ifacePlayer,
		props,
		[]string{},
	)
}

// --- Polling loop ---

func (s *Server) watchPlayback(ctx context.Context) {
	defer close(s.quitDone)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	prevStatus := "Stopped"
	prevTrackID := ""

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			st := s.player.Status()

			var status string
			switch st.State {
			case model.Playing:
				status = "Playing"
			case model.Paused:
				status = "Paused"
			default:
				status = "Stopped"
			}

			trackID := ""
			if st.Track != nil {
				trackID = st.Track.ID
			}

			changed := map[string]dbus.Variant{}

			if status != prevStatus {
				changed["PlaybackStatus"] = dbus.MakeVariant(status)
				prevStatus = status
			}

			if trackID != prevTrackID && st.Track != nil {
				prevTrackID = trackID
				t := st.Track
				meta := map[string]dbus.Variant{
					"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath("/org/mpris/MediaPlayer2/TrackList/NoTrack")),
					"xesam:title":   dbus.MakeVariant(t.Title),
					"xesam:artist":  dbus.MakeVariant([]string{t.Artist}),
					"mpris:artUrl":  dbus.MakeVariant(artURL(t)),
				}
				if t.Duration > 0 {
					meta["mpris:length"] = dbus.MakeVariant(uint64(t.Duration) / 1000)
				}
				changed["Metadata"] = dbus.MakeVariant(meta)
			}

			if len(changed) > 0 {
				s.emitPropertyChanged(changed)
			}
		}
	}
}
