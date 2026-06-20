// Package discord implements a minimal Discord Rich Presence (IPC) client
// that publishes the currently playing track as a "Listening to vimyt"
// activity. It speaks Discord's local IPC protocol directly over the
// discord-ipc-N unix socket, so it needs no third-party dependency.
package discord

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/Sadoaz/vimyt/internal/model"
)

// IPC opcodes.
const (
	opHandshake uint32 = 0
	opFrame     uint32 = 1
	opClose     uint32 = 2
)

// activityListening is the Discord activity type for "Listening to …".
const activityListening = 2

type timestamps struct {
	Start int64 `json:"start,omitempty"`
	End   int64 `json:"end,omitempty"`
}

type assets struct {
	LargeImage string `json:"large_image,omitempty"`
	LargeText  string `json:"large_text,omitempty"`
}

type button struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type activity struct {
	Type       int         `json:"type"`
	Details    string      `json:"details,omitempty"`
	State      string      `json:"state,omitempty"`
	Timestamps *timestamps `json:"timestamps,omitempty"`
	Assets     *assets     `json:"assets,omitempty"`
	Buttons    []button    `json:"buttons,omitempty"`
}

// Server polls the player and pushes Rich Presence updates to Discord.
type Server struct {
	player model.PlayerInterface
	appID  string

	mu         sync.Mutex
	enabled    bool
	showButton bool

	conn        net.Conn
	lastConnTry time.Time
	lastSig     string
	lastStart   int64 // unix-ms start used for the current progress bar

	quit     context.CancelFunc
	quitDone chan struct{}
}

// New starts a Rich Presence server. It never returns an error: if Discord is
// not running the server retries in the background and stays inert otherwise.
//
// appID is the user-supplied Discord application ID. While it is empty the
// server stays inert; the TUI sets it via SetAppID once the user has set up
// their own Discord application. The VIMYT_DISCORD_APP_ID environment variable,
// if set, overrides whatever the TUI provides.
func New(player model.PlayerInterface, enabled, showButton bool, appID string) *Server {
	if env := os.Getenv("VIMYT_DISCORD_APP_ID"); env != "" {
		appID = env
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		player:     player,
		appID:      appID,
		enabled:    enabled,
		showButton: showButton,
		quit:       cancel,
		quitDone:   make(chan struct{}),
	}
	go s.run(ctx)
	return s
}

// SetEnabled toggles whether presence is published. When disabled the current
// activity is cleared on the next tick.
func (s *Server) SetEnabled(on bool) {
	s.mu.Lock()
	s.enabled = on
	s.mu.Unlock()
}

// SetAppID swaps the Discord application ID. It drops any live connection so
// the next tick re-handshakes with the new ID. Ignored if the environment
// override is in effect. An empty id leaves the server inert.
func (s *Server) SetAppID(id string) {
	if os.Getenv("VIMYT_DISCORD_APP_ID") != "" {
		return
	}
	s.mu.Lock()
	if id != s.appID {
		s.appID = id
		if s.conn != nil {
			s.clearActivity()
			s.conn.Close()
			s.conn = nil
		}
		s.lastSig = ""
	}
	s.mu.Unlock()
}

// SetShowButton toggles the "Listen on YT Music" button.
func (s *Server) SetShowButton(on bool) {
	s.mu.Lock()
	s.showButton = on
	s.lastSig = "" // force a resend so the button appears/disappears now
	s.mu.Unlock()
}

// Close tears down the server and clears the activity.
func (s *Server) Close() {
	s.quit()
	<-s.quitDone
}

func (s *Server) run(ctx context.Context) {
	defer close(s.quitDone)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			if s.conn != nil {
				s.clearActivity()
				s.conn.Close()
				s.conn = nil
			}
			s.mu.Unlock()
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *Server) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.enabled || s.appID == "" {
		if s.conn != nil {
			s.clearActivity()
		}
		s.lastSig = ""
		return
	}

	if s.conn == nil {
		// Throttle reconnect attempts to once every ~15s.
		if time.Since(s.lastConnTry) < 15*time.Second {
			return
		}
		s.lastConnTry = time.Now()
		if err := s.connect(); err != nil {
			return
		}
	}

	act := s.activityFor(s.player.Status())
	if act == nil {
		if s.lastSig != "cleared" {
			s.clearActivity()
			s.lastSig = "cleared"
		}
		return
	}

	sig := s.signature(act)
	// Resend if anything meaningful changed, or if the real position drifted
	// from the bar we last published (e.g. after a seek).
	drift := false
	if act.Timestamps != nil {
		if d := act.Timestamps.Start - s.lastStart; d > 3000 || d < -3000 {
			drift = true
		}
	}
	if sig == s.lastSig && !drift {
		return
	}

	if err := s.setActivity(act); err != nil {
		// Connection likely dropped; drop it and retry later.
		s.conn.Close()
		s.conn = nil
		s.lastSig = ""
		return
	}
	s.lastSig = sig
	if act.Timestamps != nil {
		s.lastStart = act.Timestamps.Start
	} else {
		s.lastStart = 0
	}
}

func (s *Server) activityFor(st model.PlayerStatus) *activity {
	if st.Track == nil || st.State == model.Stopped {
		return nil
	}
	t := st.Track
	a := &activity{
		Type:    activityListening,
		Details: t.Title,
		State:   t.Artist,
	}

	art := artURL(t)
	if art != "" || t.Album != "" {
		largeText := t.Album
		if largeText == "" {
			largeText = t.Title
		}
		a.Assets = &assets{LargeImage: art, LargeText: largeText}
	}

	// Progress bar: only while actually playing and with a known duration.
	if st.State == model.Playing && t.Duration > 0 {
		now := time.Now()
		start := now.Add(-st.Position)
		a.Timestamps = &timestamps{
			Start: start.UnixMilli(),
			End:   start.Add(t.Duration).UnixMilli(),
		}
	}

	if s.showButton && t.ID != "" {
		a.Buttons = []button{{
			Label: "Listen on YT Music",
			URL:   "https://music.youtube.com/watch?v=" + t.ID,
		}}
	}
	return a
}

// signature returns a string that changes only when a resend is warranted.
func (s *Server) signature(a *activity) string {
	hasBar := a.Timestamps != nil
	return fmt.Sprintf("%s|%s|%t|%t", a.Details, a.State, hasBar, len(a.Buttons) > 0)
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

// --- IPC plumbing ---

func (s *Server) connect() error {
	conn, err := dialDiscord()
	if err != nil {
		return err
	}
	// Handshake.
	hs, _ := json.Marshal(map[string]any{"v": 1, "client_id": s.appID})
	if err := writeFrame(conn, opHandshake, hs); err != nil {
		conn.Close()
		return err
	}
	if _, _, err := readFrame(conn); err != nil {
		conn.Close()
		return err
	}
	s.conn = conn
	s.lastSig = ""
	return nil
}

func (s *Server) setActivity(a *activity) error {
	payload := map[string]any{
		"cmd": "SET_ACTIVITY",
		"args": map[string]any{
			"pid":      os.Getpid(),
			"activity": a,
		},
		"nonce": strconv.FormatInt(time.Now().UnixNano(), 10),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return writeFrame(s.conn, opFrame, data)
}

func (s *Server) clearActivity() {
	if s.conn == nil {
		return
	}
	payload := map[string]any{
		"cmd": "SET_ACTIVITY",
		"args": map[string]any{
			"pid":      os.Getpid(),
			"activity": nil,
		},
		"nonce": strconv.FormatInt(time.Now().UnixNano(), 10),
	}
	data, _ := json.Marshal(payload)
	_ = writeFrame(s.conn, opFrame, data)
}

// dialDiscord locates and connects to the discord-ipc-N unix socket.
func dialDiscord() (net.Conn, error) {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.TempDir()
	}
	dirs := []string{
		base,
		filepath.Join(base, "app", "com.discordapp.Discord"),
		filepath.Join(base, "snap.discord"),
		"/tmp",
	}
	for _, d := range dirs {
		for i := 0; i < 10; i++ {
			p := filepath.Join(d, fmt.Sprintf("discord-ipc-%d", i))
			if c, err := net.Dial("unix", p); err == nil {
				return c, nil
			}
		}
	}
	return nil, errors.New("discord ipc socket not found")
}

func writeFrame(c net.Conn, op uint32, payload []byte) error {
	buf := make([]byte, 8+len(payload))
	binary.LittleEndian.PutUint32(buf[0:4], op)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(len(payload)))
	copy(buf[8:], payload)
	_, err := c.Write(buf)
	return err
}

func readFrame(c net.Conn) (uint32, []byte, error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(c, header); err != nil {
		return 0, nil, err
	}
	op := binary.LittleEndian.Uint32(header[0:4])
	ln := binary.LittleEndian.Uint32(header[4:8])
	data := make([]byte, ln)
	if _, err := io.ReadFull(c, data); err != nil {
		return 0, nil, err
	}
	return op, data, nil
}
