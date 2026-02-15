package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// verifyLoginPreflight checks that the provided login values actually work:
// - API key can list cameras (and obtains a camera_id to test streaming)
// - org_id is present (required for streaming endpoints)
// - streaming token endpoint works
// - live m3u8 for a known camera_id returns an HLS playlist (200 + #EXTM3U)
//
// It updates cfg in-place if token refresh is required by some endpoints.
func verifyLoginPreflight(client *http.Client, cfg *Config, rf *rootFlags) error {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	// 1) Verify camera listing works (also gives us a camera_id).
	cameraID, err := preflightFetchAnyCameraID(client, cfg, rf)
	if err != nil {
		return err
	}

	// 2) org_id is required for streaming endpoints.
	if strings.TrimSpace(cfg.OrgID) == "" {
		return errors.New("login preflight failed: org id is empty (set --org-id or VERKADA_ORG_ID); required for footage streaming")
	}

	// 3) Verify we can get a streaming JWT.
	jwt, err := fetchStreamingJWT(client, *cfg, rf)
	if err != nil {
		return fmt.Errorf("login preflight failed: could not fetch streaming jwt: %w", err)
	}

	// 4) Verify the live playlist responds and looks like HLS.
	streamURL, err := buildFootageStreamM3U8URL(cfg.BaseURL, cfg.OrgID, cameraID, jwt, 0, 0, "low_res", "h264")
	if err != nil {
		return fmt.Errorf("login preflight failed: could not build stream url: %w", err)
	}
	if err := preflightCheckM3U8(client, *cfg, rf, streamURL, cameraID); err != nil {
		return err
	}

	return nil
}

func preflightFetchAnyCameraID(client *http.Client, cfg *Config, rf *rootFlags) (string, error) {
	// Page size 1 is enough for validation and avoids pulling huge orgs.
	b, ct, status, err := doCamerasDevicesRequest(client, cfg, rf, "" /* pageToken */, 1 /* pageSize */)
	if err != nil {
		return "", fmt.Errorf("login preflight failed: could not list cameras: %w", err)
	}
	if looksLikeHTML(ct, b) {
		return "", errors.New("login preflight failed: received HTML from cameras list endpoint (check --base-url is https://api(.eu|.au).verkada.com and auth headers)")
	}
	if status >= 400 {
		if pretty, ok := tryPrettyJSON(bytes.TrimSpace(b)); ok {
			return "", fmt.Errorf("login preflight failed: cameras list request failed with status %d: %s", status, strings.TrimSpace(string(pretty)))
		}
		if msg, ok := apiErrorMessage(b); ok {
			return "", fmt.Errorf("login preflight failed: cameras list request failed with status %d: %s", status, msg)
		}
		return "", fmt.Errorf("login preflight failed: cameras list request failed with status %d", status)
	}

	var out struct {
		Cameras []struct {
			CameraID string `json:"camera_id"`
		} `json:"cameras"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", fmt.Errorf("login preflight failed: cameras list returned non-JSON (check --base-url and auth): %w", err)
	}
	if len(out.Cameras) == 0 || strings.TrimSpace(out.Cameras[0].CameraID) == "" {
		return "", errors.New("login preflight failed: cameras list succeeded but returned 0 cameras; cannot validate streaming")
	}
	return strings.TrimSpace(out.Cameras[0].CameraID), nil
}

func preflightCheckM3U8(client *http.Client, cfg Config, rf *rootFlags, streamURL, cameraID string) error {
	req, err := http.NewRequest("GET", streamURL, nil)
	if err != nil {
		return fmt.Errorf("login preflight failed: invalid stream url: %w", err)
	}
	applyDefaultHeaders(req, cfg)
	if err := applyHeaderFlags(req, rf.Headers); err != nil {
		return fmt.Errorf("login preflight failed: invalid headers: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("login preflight failed: stream playlist request failed: %w", err)
	}
	defer resp.Body.Close()
	b, err := ioReadAllLimit(resp.Body, 64*1024)
	if err != nil {
		return fmt.Errorf("login preflight failed: stream playlist read failed: %w", err)
	}

	// Helpful hint for common org mismatch.
	if resp.StatusCode == 404 {
		if msg, ok := apiErrorMessage(bytes.TrimSpace(b)); ok {
			lm := strings.ToLower(msg)
			if strings.Contains(lm, "camera not found") {
				return fmt.Errorf("login preflight failed: streaming endpoint could not find camera %s under org_id %s (org_id likely incorrect)", cameraID, strings.TrimSpace(cfg.OrgID))
			}
			return fmt.Errorf("login preflight failed: streaming endpoint returned 404: %s", msg)
		}
		return fmt.Errorf("login preflight failed: streaming endpoint returned 404 (org_id/camera_id mismatch likely)")
	}
	if resp.StatusCode >= 400 {
		if pretty, ok := tryPrettyJSON(bytes.TrimSpace(b)); ok {
			return fmt.Errorf("login preflight failed: streaming endpoint failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(pretty)))
		}
		if msg, ok := apiErrorMessage(bytes.TrimSpace(b)); ok {
			return fmt.Errorf("login preflight failed: streaming endpoint failed with status %d: %s", resp.StatusCode, msg)
		}
		return fmt.Errorf("login preflight failed: streaming endpoint failed with status %d", resp.StatusCode)
	}

	// Basic HLS sniff.
	trim := bytes.TrimSpace(b)
	if !bytes.HasPrefix(trim, []byte("#EXTM3U")) {
		// Sometimes the playlist is a redirect; handle that explicitly.
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			if strings.TrimSpace(loc) != "" {
				if _, err := url.Parse(loc); err == nil {
					return fmt.Errorf("login preflight failed: streaming endpoint returned redirect to %q (unexpected)", loc)
				}
			}
		}
		return errors.New("login preflight failed: streaming endpoint returned non-m3u8 content (check org_id/camera permissions)")
	}
	return nil
}

// ioReadAllLimit reads up to limit bytes. The stream playlist should be small; this avoids huge reads on unexpected responses.
func ioReadAllLimit(r io.Reader, limit int64) ([]byte, error) {
	lr := &io.LimitedReader{R: r, N: limit}
	b, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	return b, nil
}
