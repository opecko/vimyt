package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDeviceID_GeneratesAndPersists(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)

	id1, err := LoadDeviceID()
	if err != nil {
		t.Fatalf("first LoadDeviceID: %v", err)
	}
	if !strings.HasPrefix(id1, "vimyt-") || len(id1) < 20 {
		t.Fatalf("device id looks malformed: %q", id1)
	}

	id2, err := LoadDeviceID()
	if err != nil {
		t.Fatalf("second LoadDeviceID: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("device id changed between calls: %q vs %q", id1, id2)
	}
}

func TestNowPlayingRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)

	original := NowPlaying{
		DeviceID:   "vimyt-test-123",
		DeviceName: "test-host",
		TrackID:    "abc123",
		Title:      "Bailamos",
		Artist:     "Enrique Iglesias",
		PositionMs: 12345,
		StartedAt:  "2026-04-07T15:00:00+02:00",
		Playing:    true,
	}
	if err := SaveNowPlaying(original); err != nil {
		t.Fatalf("SaveNowPlaying: %v", err)
	}

	loaded := LoadNowPlaying()
	if loaded == nil {
		t.Fatal("LoadNowPlaying returned nil")
	}
	if *loaded != original {
		t.Fatalf("round-trip mismatch:\n  original: %+v\n  loaded:   %+v", original, *loaded)
	}
}

func TestNowPlaying_SnakeCaseJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)

	if err := SaveNowPlaying(NowPlaying{
		DeviceID:   "id",
		DeviceName: "name",
		TrackID:    "tid",
		Title:      "title",
		Artist:     "artist",
		PositionMs: 7,
		StartedAt:  "2026-01-01T00:00:00Z",
		Playing:    true,
	}); err != nil {
		t.Fatalf("SaveNowPlaying: %v", err)
	}

	cfg, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(cfg, "vimyt", "now_playing.json"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("re-parse JSON: %v", err)
	}
	wantKeys := []string{
		"device_id", "device_name", "track_id", "title",
		"artist", "position_ms", "started_at", "playing",
	}
	for _, k := range wantKeys {
		if _, ok := raw[k]; !ok {
			t.Errorf("expected key %q in JSON, got keys %v", k, mapKeys(raw))
		}
	}
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
