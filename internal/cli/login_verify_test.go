package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerifyLoginPreflight_Success(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/cameras/v1/devices", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"cameras":[{"camera_id":"cam-123"}]}`)
	})
	mux.HandleFunc("/cameras/v1/footage/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jwt":"jwt-abc","expiration":1800,"expiresAt":123,"permission":["live"],"accessibleCameras":[],"accessibleSites":[]}`)
	})
	mux.HandleFunc("/stream/cameras/v1/footage/stream/stream.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-mpegURL")
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-VERSION:7\n")
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := &Config{
		BaseURL: srv.URL,
		OrgID:   "org-1",
		Auth: AuthConfig{
			APIKey: "k",
		},
		Headers: map[string]string{},
	}
	rf := &rootFlags{}
	if err := verifyLoginPreflight(srv.Client(), cfg, rf); err != nil {
		t.Fatalf("verifyLoginPreflight err = %v", err)
	}
}

func TestVerifyLoginPreflight_StreamOrgMismatch(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/cameras/v1/devices", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"cameras":[{"camera_id":"cam-123"}]}`)
	})
	mux.HandleFunc("/cameras/v1/footage/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jwt":"jwt-abc","expiration":1800,"expiresAt":123,"permission":["live"],"accessibleCameras":[],"accessibleSites":[]}`)
	})
	mux.HandleFunc("/stream/cameras/v1/footage/stream/stream.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		fmt.Fprint(w, `{"id":"pub3","message":"Camera not found. Check that your org_id and camera_id (obtained from the camera info endpoint) are correct.","data":null}`)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := &Config{
		BaseURL: srv.URL,
		OrgID:   "bad-org",
		Auth: AuthConfig{
			APIKey: "k",
		},
		Headers: map[string]string{},
	}
	rf := &rootFlags{}
	err := verifyLoginPreflight(srv.Client(), cfg, rf)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "org_id likely incorrect") {
		t.Fatalf("unexpected error: %v", err)
	}
}

