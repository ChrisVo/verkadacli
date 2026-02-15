package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type footageTokenResponseV1 struct {
	JWT               string   `json:"jwt"`
	Expiration        int      `json:"expiration"`
	ExpiresAt         int64    `json:"expiresAt"`
	Permission        []string `json:"permission"`
	AccessibleCameras []string `json:"accessibleCameras"`
	AccessibleSites   []string `json:"accessibleSites"`
}

type camerasFootageFlags struct {
	CameraID   string
	Start      string
	End        string
	Timezone   string
	Resolution string
	Codec      string
	Live       bool

	OutPath     string
	Force       bool
	Timeout     time.Duration
	PrintFFMpeg bool
}

func newCamerasFootageCmd(rf *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "footage",
		Short: "Stream or download live/historical camera footage",
	}
	cmd.AddCommand(newCamerasFootageURLCmd(rf))
	cmd.AddCommand(newCamerasFootageDownloadCmd(rf))
	return cmd
}

func newCamerasFootageURLCmd(rf *rootFlags) *cobra.Command {
	var f camerasFootageFlags

	cmd := &cobra.Command{
		Use:   "url",
		Short: "Print an HLS (m3u8) URL for live or historical footage",
		Example: strings.TrimSpace(`
  verkada cameras footage url --camera-id CAM123 --start 2026-02-15T14:00:00Z --end 2026-02-15T14:10:00Z
  verkada cameras footage url --camera-id CAM123 --live
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := effectiveConfig(*rf)
			if err != nil {
				return err
			}
			if strings.TrimSpace(f.CameraID) == "" {
				return errors.New("--camera-id is required")
			}

			client := &http.Client{Timeout: f.Timeout}
			if _, err := ensureOrgID(client, &cfg, rf); err != nil {
				return err
			}
			if strings.TrimSpace(cfg.OrgID) == "" {
				return errors.New("org id is empty (set in config, VERKADA_ORG_ID, or --org-id)")
			}

			startTime, endTime, err := resolveStreamTimes(f)
			if err != nil {
				return err
			}

			jwt, err := fetchStreamingJWT(&http.Client{Timeout: f.Timeout}, cfg, rf)
			if err != nil {
				return err
			}

			u, err := buildFootageStreamM3U8URL(cfg.BaseURL, cfg.OrgID, f.CameraID, jwt, startTime, endTime, f.Resolution, f.Codec)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), u)
			return nil
		},
	}

	addFootageCommonFlags(cmd, &f)
	return cmd
}

func newCamerasFootageDownloadCmd(rf *rootFlags) *cobra.Command {
	var f camerasFootageFlags

	cmd := &cobra.Command{
		Use:   "download",
		Short: "Download an MP4 clip via HLS using ffmpeg (requires ffmpeg installed)",
		Example: strings.TrimSpace(`
  verkada cameras footage download --camera-id CAM123 --start 2026-02-15T14:00:00Z --end 2026-02-15T14:10:00Z --out clip.mp4
  verkada cameras footage download --camera-id CAM123 --start "2026-02-15 06:00:00" --end "2026-02-15 06:05:00" --tz America/Los_Angeles --out clip.mp4
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := effectiveConfig(*rf)
			if err != nil {
				return err
			}
			if strings.TrimSpace(f.CameraID) == "" {
				return errors.New("--camera-id is required")
			}
			if strings.TrimSpace(f.OutPath) == "" {
				return errors.New("--out is required")
			}

			startTime, endTime, err := resolveStreamTimes(f)
			if err != nil {
				return err
			}
			if startTime == 0 || endTime == 0 {
				return errors.New("download requires historical times; provide --start and --end (or omit --live)")
			}

			if _, err := exec.LookPath("ffmpeg"); err != nil {
				return errors.New("ffmpeg not found in PATH; install ffmpeg or use `verkada cameras footage url ...` and download with your own HLS tool")
			}

			client := &http.Client{Timeout: f.Timeout}
			if _, err := ensureOrgID(client, &cfg, rf); err != nil {
				return err
			}
			if strings.TrimSpace(cfg.OrgID) == "" {
				return errors.New("org id is empty (set in config, VERKADA_ORG_ID, or --org-id)")
			}
			jwt, err := fetchStreamingJWT(client, cfg, rf)
			if err != nil {
				return err
			}

			streamURL, err := buildFootageStreamM3U8URL(cfg.BaseURL, cfg.OrgID, f.CameraID, jwt, startTime, endTime, f.Resolution, f.Codec)
			if err != nil {
				return err
			}

			playlist, err := fetchText(client, streamURL, cfg, rf)
			if err != nil {
				return err
			}

			rewriteURL, _ := url.Parse(streamURL)
			baseQuery := rewriteURL.Query()
			rewritten, err := rewriteM3U8(playlist, rewriteURL, baseQuery)
			if err != nil {
				return err
			}

			tmp, err := os.CreateTemp("", "verkada_footage_*.m3u8")
			if err != nil {
				return err
			}
			tmpPath := tmp.Name()
			_ = tmp.Close()
			defer os.Remove(tmpPath)

			if err := os.WriteFile(tmpPath, rewritten, 0o600); err != nil {
				return err
			}

			if dir := filepath.Dir(f.OutPath); dir != "." {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return err
				}
			}

			argsFF := []string{
				"-hide_banner",
				"-loglevel", "error",
				"-protocol_whitelist", "file,http,https,tcp,tls,crypto",
				"-allowed_extensions", "ALL",
			}
			if f.Force {
				argsFF = append(argsFF, "-y")
			} else {
				argsFF = append(argsFF, "-n")
			}
			argsFF = append(argsFF,
				"-i", tmpPath,
				"-c", "copy",
				f.OutPath,
			)

			if f.PrintFFMpeg {
				fmt.Fprintln(cmd.OutOrStdout(), "ffmpeg "+shellQuoteArgs(argsFF))
				return nil
			}

			c := exec.Command("ffmpeg", argsFF...)
			c.Stdout = cmd.ErrOrStderr()
			c.Stderr = cmd.ErrOrStderr()
			if err := c.Run(); err != nil {
				return fmt.Errorf("ffmpeg failed: %w", err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s\n", f.OutPath)
			return nil
		},
	}

	addFootageCommonFlags(cmd, &f)
	cmd.Flags().StringVarP(&f.OutPath, "out", "o", "", "Write MP4 to file (required)")
	cmd.Flags().BoolVar(&f.Force, "force", false, "Overwrite output file if it exists")
	cmd.Flags().BoolVar(&f.PrintFFMpeg, "print-ffmpeg", false, "Print the ffmpeg command that would be run, then exit")
	return cmd
}

func addFootageCommonFlags(cmd *cobra.Command, f *camerasFootageFlags) {
	cmd.Flags().StringVar(&f.CameraID, "camera-id", "", "Camera ID (required)")
	cmd.Flags().StringVar(&f.Start, "start", "", "Start time for historical footage. Accepts Unix seconds, RFC3339, RFC3339 without timezone, or 'YYYY-MM-DD HH:MM:SS'.")
	cmd.Flags().StringVar(&f.End, "end", "", "End time for historical footage. Accepts Unix seconds, RFC3339, RFC3339 without timezone, or 'YYYY-MM-DD HH:MM:SS'.")
	cmd.Flags().StringVar(&f.Timezone, "tz", "local", "Timezone used for naive --start/--end values.")
	cmd.Flags().BoolVar(&f.Live, "live", false, "Stream live footage (equivalent to start_time=0,end_time=0)")
	cmd.Flags().StringVar(&f.Resolution, "resolution", "low_res", "Resolution: low_res|high_res")
	cmd.Flags().StringVar(&f.Codec, "codec", "hevc", "Codec: hevc|h264 (depending on camera/availability)")
	cmd.Flags().DurationVar(&f.Timeout, "timeout", 30*time.Second, "HTTP timeout")
}

func resolveStreamTimes(f camerasFootageFlags) (startTime int64, endTime int64, err error) {
	if f.Live {
		return 0, 0, nil
	}
	startRaw := strings.TrimSpace(f.Start)
	endRaw := strings.TrimSpace(f.End)
	if startRaw == "" && endRaw == "" {
		// Default to live if no window is provided.
		return 0, 0, nil
	}
	if startRaw == "" || endRaw == "" {
		return 0, 0, errors.New("both --start and --end are required for historical footage")
	}
	st, err := parseThumbnailTimestamp(startRaw, f.Timezone)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid --start: %w", err)
	}
	et, err := parseThumbnailTimestamp(endRaw, f.Timezone)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid --end: %w", err)
	}
	if st <= 0 || et <= 0 {
		return 0, 0, errors.New("historical --start/--end must be positive unix timestamps")
	}
	if et <= st {
		return 0, 0, errors.New("--end must be after --start")
	}
	if (et - st) > 3600 {
		return 0, 0, errors.New("historical window too large: end-start must be <= 3600 seconds (1 hour)")
	}
	return st, et, nil
}

func buildFootageTokenURL(baseURL string) (string, error) {
	bu, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	pu, err := url.Parse("/cameras/v1/footage/token")
	if err != nil {
		return "", err
	}
	return bu.ResolveReference(pu).String(), nil
}

func fetchStreamingJWT(client *http.Client, cfg Config, rf *rootFlags) (string, error) {
	tu, err := buildFootageTokenURL(cfg.BaseURL)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("GET", tu, nil)
	if err != nil {
		return "", err
	}
	applyDefaultHeaders(req, cfg)
	if err := applyHeaderFlags(req, rf.Headers); err != nil {
		return "", err
	}
	applyBestEffortAuth(req, cfg) // ensures x-api-key is present when configured

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if looksLikeHTML(resp.Header.Get("Content-Type"), b) {
		return "", errors.New("received HTML from footage token endpoint (check --base-url is https://api(.eu|.au).verkada.com and auth header x-api-key)")
	}
	if resp.StatusCode >= 400 {
		if pretty, ok := tryPrettyJSON(b); ok {
			return "", fmt.Errorf("footage token request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(pretty)))
		}
		return "", fmt.Errorf("footage token request failed with status %d", resp.StatusCode)
	}

	var out footageTokenResponseV1
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.JWT) == "" {
		return "", errors.New("footage token response missing jwt field")
	}
	return out.JWT, nil
}

func buildFootageStreamM3U8URL(baseURL, orgID, cameraID, jwt string, startTime, endTime int64, resolution, codec string) (string, error) {
	bu, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	pu, err := url.Parse("/stream/cameras/v1/footage/stream/stream.m3u8")
	if err != nil {
		return "", err
	}
	u := bu.ResolveReference(pu)
	q := u.Query()
	q.Set("org_id", strings.TrimSpace(orgID))
	q.Set("camera_id", strings.TrimSpace(cameraID))
	q.Set("jwt", strings.TrimSpace(jwt))
	q.Set("type", "stream")

	if startTime != 0 || endTime != 0 {
		q.Set("start_time", strconv.FormatInt(startTime, 10))
		q.Set("end_time", strconv.FormatInt(endTime, 10))
	} else {
		q.Set("start_time", "0")
		q.Set("end_time", "0")
	}

	resolution = strings.TrimSpace(resolution)
	if resolution == "" {
		resolution = "low_res"
	}
	switch resolution {
	case "low_res", "high_res":
		// ok
	default:
		return "", fmt.Errorf("invalid --resolution %q (expected low_res or high_res)", resolution)
	}
	q.Set("resolution", resolution)

	codec = strings.TrimSpace(codec)
	if codec == "" {
		codec = "hevc"
	}
	// Don't validate too aggressively; docs default to hevc but some orgs may prefer h264.
	q.Set("codec", codec)

	u.RawQuery = q.Encode()
	return u.String(), nil
}

func fetchText(client *http.Client, reqURL string, cfg Config, rf *rootFlags) ([]byte, error) {
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	applyDefaultHeaders(req, cfg)
	if err := applyHeaderFlags(req, rf.Headers); err != nil {
		return nil, err
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if rf.Debug {
		fmt.Fprintf(os.Stderr, "HTTP %s %s -> %d (%s)\n", req.Method, req.URL.String(), resp.StatusCode, time.Since(start))
	}
	if looksLikeHTML(resp.Header.Get("Content-Type"), b) {
		return nil, fmt.Errorf("received HTML instead of m3u8 (check org_id/camera_id and base URL)")
	}
	if resp.StatusCode >= 400 {
		trim := bytes.TrimSpace(b)
		if pretty, ok := tryPrettyJSON(trim); ok {
			return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(pretty)))
		}
		return nil, fmt.Errorf("request failed with status %d", resp.StatusCode)
	}
	return b, nil
}

func rewriteM3U8(in []byte, playlistURL *url.URL, requiredQuery url.Values) ([]byte, error) {
	// Rewrite relative URIs to absolute and ensure required query params (org_id/camera_id/jwt/etc) are present
	// on segment/key URIs. This makes tooling like ffmpeg more reliable across HLS variants.
	lines := strings.Split(string(in), "\n")
	var out strings.Builder
	out.Grow(len(in) + 256)

	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" {
			out.WriteString(line)
		} else if strings.HasPrefix(trim, "#") {
			rewritten, err := rewriteM3U8TagLine(line, playlistURL, requiredQuery)
			if err != nil {
				return nil, fmt.Errorf("invalid m3u8 tag on line %d: %w", i+1, err)
			}
			out.WriteString(rewritten)
		} else {
			u, err := url.Parse(trim)
			if err != nil {
				return nil, fmt.Errorf("invalid m3u8 uri on line %d: %w", i+1, err)
			}
			if !u.IsAbs() {
				u = playlistURL.ResolveReference(u)
			}
			q := u.Query()
			for k, vals := range requiredQuery {
				if q.Has(k) {
					continue
				}
				for _, v := range vals {
					q.Add(k, v)
				}
			}
			u.RawQuery = q.Encode()
			out.WriteString(u.String())
		}

		// Preserve trailing newline behavior.
		if i != len(lines)-1 {
			out.WriteByte('\n')
		}
	}

	b := []byte(out.String())
	if len(b) == 0 || b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	return b, nil
}

func rewriteM3U8TagLine(line string, playlistURL *url.URL, requiredQuery url.Values) (string, error) {
	// Some HLS tags embed URIs inside the tag line, e.g.:
	// - #EXT-X-KEY:...URI="key.key"...
	// - #EXT-X-MAP:URI="init.mp4"...
	// - #EXT-X-MEDIA:URI="sub.m3u8"...
	//
	// Rewrite any URI="..." substrings.
	const needle = `URI="`
	out := line
	pos := 0
	for {
		idx := strings.Index(out[pos:], needle)
		if idx == -1 {
			return out, nil
		}
		idx += pos
		start := idx + len(needle)
		end := strings.Index(out[start:], `"`)
		if end == -1 {
			return "", errors.New("unterminated URI=\"...\"")
		}
		end = start + end
		raw := out[start:end]

		u, err := url.Parse(raw)
		if err != nil {
			return "", err
		}
		if !u.IsAbs() {
			u = playlistURL.ResolveReference(u)
		}
		q := u.Query()
		for k, vals := range requiredQuery {
			if q.Has(k) {
				continue
			}
			for _, v := range vals {
				q.Add(k, v)
			}
		}
		u.RawQuery = q.Encode()

		repl := u.String()
		out = out[:start] + repl + out[end:]
		pos = start + len(repl)
	}
}

func shellQuoteArgs(args []string) string {
	// Minimal quoting for debug printing; uses single quotes and escapes existing ones.
	var parts []string
	for _, a := range args {
		if a == "" {
			parts = append(parts, "''")
			continue
		}
		if !strings.ContainsAny(a, " \t\n'\"\\$`") {
			parts = append(parts, a)
			continue
		}
		parts = append(parts, "'"+strings.ReplaceAll(a, "'", "'\\''")+"'")
	}
	return strings.Join(parts, " ")
}
