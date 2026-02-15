package cli

import (
	"strings"
	"testing"
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
