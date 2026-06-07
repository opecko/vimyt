package model

import "testing"

func TestSanitizeTextRemovesTerminalHostileRunes(t *testing.T) {
	input := "Break The Rules\uFE0F\u200D(Hardstyle)\n\tNow"
	got := SanitizeText(input)
	want := "Break The Rules(Hardstyle) Now"
	if got != want {
		t.Fatalf("SanitizeText() = %q, want %q", got, want)
	}
}

func TestSanitizeTrackCleansTitleAndArtist(t *testing.T) {
	track := SanitizeTrack(Track{
		ID:     "id",
		Title:  "A\u200bTitle",
		Artist: "An\u0007Artist",
	})

	if track.Title != "ATitle" {
		t.Fatalf("title = %q, want %q", track.Title, "ATitle")
	}
	if track.Artist != "AnArtist" {
		t.Fatalf("artist = %q, want %q", track.Artist, "AnArtist")
	}
}
