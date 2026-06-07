package model

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type NowPlaying struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	TrackID    string `json:"track_id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	PositionMs int64  `json:"position_ms"`
	StartedAt  string `json:"started_at"`
	Playing    bool   `json:"playing"`
}

func nowPlayingPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, "vimyt", "now_playing.json"), nil
}

func deviceIDPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, "vimyt", "device_id.txt"), nil
}

func LoadDeviceID() (string, error) {
	path, err := deviceIDPath()
	if err != nil {
		return "", err
	}
	if data, err := os.ReadFile(path); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			return id, nil
		}
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("device id rand: %w", err)
	}
	id := "vimyt-" + hex.EncodeToString(buf)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o644); err != nil {
		return "", err
	}
	return id, nil
}

func LoadNowPlaying() *NowPlaying {
	path, err := nowPlayingPath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var np NowPlaying
	if err := json.Unmarshal(data, &np); err != nil {
		return nil
	}
	return &np
}

func SaveNowPlaying(np NowPlaying) error {
	path, err := nowPlayingPath()
	if err != nil {
		return err
	}
	if np.StartedAt == "" {
		np.StartedAt = time.Now().Format(time.RFC3339Nano)
	}
	data, err := json.MarshalIndent(np, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func DeviceName() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "vimyt"
}
