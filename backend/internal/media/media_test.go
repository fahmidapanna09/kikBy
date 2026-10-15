package media

import "testing"

func TestAllowedMimeTypes_RejectsWrongKind(t *testing.T) {
	if allowedMimeTypes[KindImage]["application/pdf"] {
		t.Fatal("application/pdf should not be allowed for kind=image")
	}
	if !allowedMimeTypes[KindDocument]["application/pdf"] {
		t.Fatal("application/pdf should be allowed for kind=document")
	}
	if allowedMimeTypes[KindVoice]["image/jpeg"] {
		t.Fatal("image/jpeg should not be allowed for kind=voice")
	}
}

func TestExtensionFor(t *testing.T) {
	cases := map[string]string{
		"image/jpeg":      ".jpg",
		"image/png":       ".png",
		"video/mp4":       ".mp4",
		"application/pdf": ".pdf",
		"audio/amr":       "", // not in the small explicit extension map; caller falls back to no extension
	}
	for mime, want := range cases {
		if got := extensionFor(mime); got != want {
			t.Errorf("extensionFor(%q) = %q, want %q", mime, got, want)
		}
	}
}

func TestLimits_MaxFor(t *testing.T) {
	l := Limits{MaxImageBytes: 1, MaxVideoBytes: 2, MaxAudioBytes: 3, MaxDocumentBytes: 4}
	if l.maxFor(KindImage) != 1 || l.maxFor(KindVideo) != 2 || l.maxFor(KindDocument) != 4 {
		t.Fatal("maxFor did not return the expected per-kind limit")
	}
	if l.maxFor(KindVoice) != l.maxFor(KindAudio) {
		t.Fatal("voice notes should share the audio size limit")
	}
}

func TestScaledDimensions_PreservesAspectRatio(t *testing.T) {
	w, h := scaledDimensions(4000, 2000, 320)
	if w != 320 {
		t.Fatalf("expected width to be capped at 320, got %d", w)
	}
	if h != 160 {
		t.Fatalf("expected height to scale proportionally to 160, got %d", h)
	}

	// Already smaller than max on both axes: unchanged.
	w2, h2 := scaledDimensions(100, 80, 320)
	if w2 != 100 || h2 != 80 {
		t.Fatalf("expected dimensions under the cap to be left unchanged, got %dx%d", w2, h2)
	}
}

func TestThumbnailKeyFor(t *testing.T) {
	got := thumbnailKeyFor("media/user-1/abc123.jpg")
	want := "media/user-1/abc123_thumb.jpg"
	if got != want {
		t.Errorf("thumbnailKeyFor = %q, want %q", got, want)
	}
}
