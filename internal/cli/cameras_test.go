package cli

import (
	"strings"
	"testing"
	"time"
)

func TestBuildCamerasThumbnailURL(t *testing.T) {
	u, err := buildCamerasThumbnailURL("https://api.verkada.com", "CAM123", 1736893300, "hi-res")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(u, "https://api.verkada.com/cameras/v1/footage/thumbnails?") {
		t.Fatalf("unexpected url: %s", u)
	}
	// Order isn't guaranteed; just check required params.
	if !strings.Contains(u, "camera_id=CAM123") {
		t.Fatalf("missing camera_id: %s", u)
	}
	if !strings.Contains(u, "timestamp=1736893300") {
		t.Fatalf("missing timestamp: %s", u)
	}
	if !strings.Contains(u, "resolution=hi-res") {
		t.Fatalf("missing resolution: %s", u)
	}
}

func TestFormatCameraListText_Array(t *testing.T) {
	body := []byte(`[
  {"camera_id":"CAM1","name":"Front Door","site":"HQ","model":"CB52","serial_number":"S1","status":"online"},
  {"camera_id":"CAM2","name":"Lobby","site":"HQ","model":"CB52","serial_number":"S2","status":"offline"}
]`)
	s, err := formatCameraListText(body, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(s, "CAM1") || !strings.Contains(s, "Front Door") {
		t.Fatalf("unexpected output: %q", s)
	}
}

func TestFormatCameraListText_EnvelopeDevices(t *testing.T) {
	body := []byte(`{"devices":[{"cameraId":"CAM9","deviceName":"Side","siteName":"SF"}]}`)
	s, err := formatCameraListText(body, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(s, "CAM9") || !strings.Contains(s, "Side") || !strings.Contains(s, "SF") {
		t.Fatalf("unexpected output: %q", s)
	}
}

func TestDecideThumbnailOutput_Piped_Default(t *testing.T) {
	writeStdout, viewEnabled, err := decideThumbnailOutput(false, false, "", false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !writeStdout || viewEnabled {
		t.Fatalf("unexpected plan: write=%v view=%v", writeStdout, viewEnabled)
	}
}

func TestDecideThumbnailOutput_TTY_InlineSupported_DefaultsToView(t *testing.T) {
	writeStdout, viewEnabled, err := decideThumbnailOutput(true, true, "", false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if writeStdout || !viewEnabled {
		t.Fatalf("unexpected plan: write=%v view=%v", writeStdout, viewEnabled)
	}
}

func TestDecideThumbnailOutput_TTY_NoInline_Errors(t *testing.T) {
	_, _, err := decideThumbnailOutput(true, false, "", false)
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestDecideThumbnailOutput_TTY_ViewFlag_SuppressesStdout(t *testing.T) {
	writeStdout, viewEnabled, err := decideThumbnailOutput(true, false, "", true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if writeStdout || !viewEnabled {
		t.Fatalf("unexpected plan: write=%v view=%v", writeStdout, viewEnabled)
	}
}

func TestParseThumbnailTimestamp_DefaultsToNow(t *testing.T) {
	before := time.Now().Unix() - 2
	got, err := parseThumbnailTimestamp("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := time.Now().Unix() + 2
	if got < before || got > after {
		t.Fatalf("unexpected timestamp: %d (expected within current window)", got)
	}
}

func TestParseThumbnailTimestamp_AcceptsUnixTimestamp(t *testing.T) {
	const ts = int64(1736893300)
	got, err := parseThumbnailTimestamp("1736893300", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ts {
		t.Fatalf("unexpected timestamp: %d", got)
	}
}

func TestParseThumbnailTimestamp_AcceptsRFC3339(t *testing.T) {
	const expected = int64(1739573400) // 2025-02-15T14:30:00Z
	got, err := parseThumbnailTimestamp("2025-02-15T14:30:00Z", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Fatalf("unexpected timestamp: %d", got)
	}
}

func TestParseThumbnailTimestamp_AcceptsLocalTime(t *testing.T) {
	const sample = "2025-02-15 14:30:00"
	got, err := parseThumbnailTimestamp(sample, "local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", sample, time.Local)
	if err != nil {
		t.Fatalf("time parse failed: %v", err)
	}
	if got != parsed.Unix() {
		t.Fatalf("unexpected timestamp: %d", got)
	}
}

func TestParseThumbnailTimestamp_AcceptsTimezone(t *testing.T) {
	const sample = "2025-02-15 08:00:00"
	got, err := parseThumbnailTimestamp(sample, "America/Los_Angeles")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("unexpected tz load error: %v", err)
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", sample, loc)
	if err != nil {
		t.Fatalf("unexpected parse: %v", err)
	}
	if got != parsed.Unix() {
		t.Fatalf("unexpected timestamp: %d", got)
	}
}

func TestParseTimestampLocation_RejectsInvalidTimezone(t *testing.T) {
	if _, _, err := parseTimestampLocation("Not/AZone"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseThumbnailTimestamp_RejectsInvalidInput(t *testing.T) {
	if _, err := parseThumbnailTimestamp("not-a-timestamp", ""); err == nil {
		t.Fatalf("expected error")
	}
}
