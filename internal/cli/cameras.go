package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// NewCamerasCmd groups camera-related typed endpoints.
func NewCamerasCmd(rf *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cameras",
		Short: "Camera endpoints",
	}

	cmd.AddCommand(newCamerasListCmd(rf))
	cmd.AddCommand(newCamerasGetCmd(rf))
	cmd.AddCommand(newCamerasLabelCmd(rf))
	cmd.AddCommand(newCamerasThumbnailCmd(rf))
	return cmd
}

func newCamerasListCmd(rf *rootFlags) *cobra.Command {
	var timeout time.Duration
	var pageSize int
	var pageToken string
	var all bool
	var wide bool
	var cameraID string
	var q string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List cameras in the org",
		Example: strings.TrimSpace(`
  verkada cameras list
  verkada cameras list --page-size 200
  verkada cameras list --all
  verkada --profile eu cameras list --output json
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := effectiveConfig(*rf)
			if err != nil {
				return err
			}

			client := &http.Client{Timeout: timeout}
			out := cmd.OutOrStdout()
			needsProcessing := strings.TrimSpace(cameraID) != "" || strings.TrimSpace(q) != ""

			// If not fetching all pages, behave as pass-through (pretty JSON when requested),
			// otherwise aggregate into a single {cameras:[...]} response.
			if !all && !needsProcessing {
				b, _, status, err := doCamerasDevicesRequest(client, &cfg, rf, pageToken, pageSize)
				if err != nil {
					return err
				}
				if looksLikeHTML("", b) {
					return fmt.Errorf("received HTML instead of camera JSON (check --base-url is https://api(.eu|.au).verkada.com and auth headers x-api-key / x-verkada-auth)")
				}
				if status >= 400 {
					if pretty, ok := tryPrettyJSON(b); ok {
						_, _ = out.Write(pretty)
					} else {
						_, _ = out.Write(b)
						if len(b) == 0 || b[len(b)-1] != '\n' {
							fmt.Fprintln(out)
						}
					}
					return fmt.Errorf("request failed with status %d", status)
				}
				if rf.Output == "json" {
					if pretty, ok := tryPrettyJSON(b); ok {
						_, _ = out.Write(pretty)
					} else {
						_, _ = out.Write(b)
						if len(b) == 0 || b[len(b)-1] != '\n' {
							fmt.Fprintln(out)
						}
					}
					return nil
				}

				s, err := formatCameraListText(b, wide, cfg.Labels)
				if err != nil {
					_, _ = out.Write(b)
					if len(b) == 0 || b[len(b)-1] != '\n' {
						fmt.Fprintln(out)
					}
					return nil
				}
				fmt.Fprint(out, s)
				return nil
			}

			agg := make([]map[string]any, 0, 128)
			next := pageToken
			for {
				b, _, status, err := doCamerasDevicesRequest(client, &cfg, rf, next, pageSize)
				if err != nil {
					return err
				}
				if looksLikeHTML("", b) {
					return fmt.Errorf("received HTML instead of camera JSON (check --base-url is https://api(.eu|.au).verkada.com and auth headers x-api-key / x-verkada-auth)")
				}
				if status >= 400 {
					if pretty, ok := tryPrettyJSON(b); ok {
						_, _ = out.Write(pretty)
					} else {
						_, _ = out.Write(b)
						if len(b) == 0 || b[len(b)-1] != '\n' {
							fmt.Fprintln(out)
						}
					}
					return fmt.Errorf("request failed with status %d", status)
				}

				cams, token, err := extractCamerasAndNextToken(b)
				if err != nil {
					// If we can't parse it, fall back to printing first page and stop.
					if rf.Output == "json" {
						if pretty, ok := tryPrettyJSON(b); ok {
							_, _ = out.Write(pretty)
						} else {
							_, _ = out.Write(b)
							if len(b) == 0 || b[len(b)-1] != '\n' {
								fmt.Fprintln(out)
							}
						}
						return nil
					}
					s, ferr := formatCameraListText(b, wide, cfg.Labels)
					if ferr != nil {
						_, _ = out.Write(b)
						if len(b) == 0 || b[len(b)-1] != '\n' {
							fmt.Fprintln(out)
						}
						return nil
					}
					fmt.Fprint(out, s)
					return nil
				}

				agg = append(agg, cams...)
				if strings.TrimSpace(token) == "" {
					break
				}
				next = token
			}

			if needsProcessing {
				agg = filterCameras(agg, cameraID, q, cfg.Labels)
			}

			if rf.Output == "json" {
				blob, err := json.MarshalIndent(map[string]any{"cameras": agg}, "", "  ")
				if err != nil {
					return err
				}
				blob = append(blob, '\n')
				_, _ = out.Write(blob)
				return nil
			}

			blob, err := json.Marshal(map[string]any{"cameras": agg})
			if err != nil {
				return err
			}
			s, err := formatCameraListText(blob, wide, cfg.Labels)
			if err != nil {
				// Fallback to JSON if text formatting fails.
				pretty, _ := json.MarshalIndent(map[string]any{"cameras": agg}, "", "  ")
				pretty = append(pretty, '\n')
				_, _ = out.Write(pretty)
				return nil
			}
			fmt.Fprint(out, s)
			return nil
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "HTTP timeout")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "Page size (default 100, max 200)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "Pagination token to start from")
	cmd.Flags().BoolVar(&all, "all", false, "Fetch all pages")
	cmd.Flags().BoolVar(&wide, "wide", false, "Include more columns in text output")
	cmd.Flags().StringVar(&cameraID, "camera-id", "", "Filter by camera ID (exact match)")
	cmd.Flags().StringVar(&q, "q", "", "Filter by substring match across id/name/site/label")
	return cmd
}

func newCamerasGetCmd(rf *rootFlags) *cobra.Command {
	var timeout time.Duration
	var pageSize int

	cmd := &cobra.Command{
		Use:   "get CAMERA_ID",
		Short: "Get details for a single camera (fetched from the list endpoint)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cameraID := strings.TrimSpace(args[0])
			if cameraID == "" {
				return errors.New("camera_id is empty")
			}

			cfg, err := effectiveConfig(*rf)
			if err != nil {
				return err
			}

			client := &http.Client{Timeout: timeout}
			out := cmd.OutOrStdout()

			next := ""
			for {
				b, _, status, err := doCamerasDevicesRequest(client, &cfg, rf, next, pageSize)
				if err != nil {
					return err
				}
				if looksLikeHTML("", b) {
					return fmt.Errorf("received HTML instead of camera JSON (check --base-url is https://api(.eu|.au).verkada.com and auth headers x-api-key / x-verkada-auth)")
				}
				if status >= 400 {
					if pretty, ok := tryPrettyJSON(b); ok {
						_, _ = out.Write(pretty)
					} else {
						_, _ = out.Write(b)
						if len(b) == 0 || b[len(b)-1] != '\n' {
							fmt.Fprintln(out)
						}
					}
					return fmt.Errorf("request failed with status %d", status)
				}

				cams, token, err := extractCamerasAndNextToken(b)
				if err != nil {
					// If we can't parse the response, just pass it through.
					if rf.Output == "json" {
						if pretty, ok := tryPrettyJSON(b); ok {
							_, _ = out.Write(pretty)
						} else {
							_, _ = out.Write(b)
							if len(b) == 0 || b[len(b)-1] != '\n' {
								fmt.Fprintln(out)
							}
						}
						return nil
					}
					_, _ = out.Write(b)
					if len(b) == 0 || b[len(b)-1] != '\n' {
						fmt.Fprintln(out)
					}
					return nil
				}

				for _, c := range cams {
					id := pickString(c, "camera_id", "cameraId", "cameraID", "id")
					if id != cameraID {
						continue
					}

					if rf.Output == "json" {
						blob, err := json.MarshalIndent(c, "", "  ")
						if err != nil {
							return err
						}
						blob = append(blob, '\n')
						_, _ = out.Write(blob)
						return nil
					}

					blob, err := json.Marshal(map[string]any{"cameras": []map[string]any{c}})
					if err != nil {
						return err
					}
					s, err := formatCameraListText(blob, true, cfg.Labels)
					if err != nil {
						blob, _ := json.MarshalIndent(c, "", "  ")
						blob = append(blob, '\n')
						_, _ = out.Write(blob)
						return nil
					}
					fmt.Fprint(out, s)
					return nil
				}

				if strings.TrimSpace(token) == "" {
					break
				}
				next = token
			}

			return fmt.Errorf("camera %q not found", cameraID)
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "HTTP timeout")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "Page size (default 100, max 200)")
	return cmd
}

func newCamerasLabelCmd(rf *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label",
		Short: "Manage local camera labels (stored in the config profile)",
	}
	cmd.AddCommand(newCamerasLabelSetCmd(rf))
	cmd.AddCommand(newCamerasLabelRmCmd(rf))
	cmd.AddCommand(newCamerasLabelListCmd(rf))
	return cmd
}

func newCamerasLabelSetCmd(rf *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set CAMERA_ID LABEL",
		Short: "Set a local label for a camera",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cameraID := strings.TrimSpace(args[0])
			label := strings.TrimSpace(args[1])
			if cameraID == "" {
				return errors.New("camera_id is empty")
			}
			if label == "" {
				return errors.New("label is empty")
			}

			p, err := resolveConfigPath(rf.ConfigPath)
			if err != nil {
				return err
			}
			cf, err := loadConfig(p)
			if err != nil {
				return err
			}

			profileName := firstNonEmpty(rf.Profile, envOr("VERKADA_PROFILE", ""), cf.CurrentProfile, "default")
			profile, ok := cf.Profiles[profileName]
			if !ok {
				return fmt.Errorf("profile %q not found in %s", profileName, p)
			}
			if profile.Labels == nil {
				profile.Labels = &LocalLabels{Cameras: map[string]string{}}
			} else if profile.Labels.Cameras == nil {
				profile.Labels.Cameras = map[string]string{}
			}

			profile.Labels.Cameras[cameraID] = label
			cf.Profiles[profileName] = profile
			if err := writeConfig(p, cf); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "label[%s]=%s\n", cameraID, label)
			return nil
		},
	}
	return cmd
}

func newCamerasLabelRmCmd(rf *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm CAMERA_ID",
		Short: "Remove a local label for a camera",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cameraID := strings.TrimSpace(args[0])
			if cameraID == "" {
				return errors.New("camera_id is empty")
			}

			p, err := resolveConfigPath(rf.ConfigPath)
			if err != nil {
				return err
			}
			cf, err := loadConfig(p)
			if err != nil {
				return err
			}

			profileName := firstNonEmpty(rf.Profile, envOr("VERKADA_PROFILE", ""), cf.CurrentProfile, "default")
			profile, ok := cf.Profiles[profileName]
			if !ok {
				return fmt.Errorf("profile %q not found in %s", profileName, p)
			}

			if profile.Labels != nil && profile.Labels.Cameras != nil {
				delete(profile.Labels.Cameras, cameraID)
			}
			cf.Profiles[profileName] = profile
			if err := writeConfig(p, cf); err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}

func newCamerasLabelListCmd(rf *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List local camera labels for the selected profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := resolveConfigPath(rf.ConfigPath)
			if err != nil {
				return err
			}
			cf, err := loadConfig(p)
			if err != nil {
				return err
			}

			profileName := firstNonEmpty(rf.Profile, envOr("VERKADA_PROFILE", ""), cf.CurrentProfile, "default")
			profile, ok := cf.Profiles[profileName]
			if !ok {
				return fmt.Errorf("profile %q not found in %s", profileName, p)
			}

			if profile.Labels == nil || len(profile.Labels.Cameras) == 0 {
				return nil
			}

			type kv struct {
				k string
				v string
			}
			items := make([]kv, 0, len(profile.Labels.Cameras))
			for k, v := range profile.Labels.Cameras {
				items = append(items, kv{k: k, v: v})
			}
			sort.Slice(items, func(i, j int) bool { return items[i].k < items[j].k })
			for _, it := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", it.k, it.v)
			}
			return nil
		},
	}
	return cmd
}

type camerasThumbnailFlags struct {
	CameraID   string
	Timestamp  int64
	Resolution string

	OutPath string
	View    bool
	Timeout time.Duration
}

func newCamerasThumbnailCmd(rf *rootFlags) *cobra.Command {
	var f camerasThumbnailFlags

	cmd := &cobra.Command{
		Use:   "thumbnail",
		Short: "Get a thumbnail image (JPEG) for a camera at/near a timestamp",
		Long: strings.TrimSpace(`
Returns a low-resolution or high-resolution thumbnail from a specified camera at or near a specified time.

The response body is raw binary JPEG data. By default, this command writes the JPEG to stdout.
Use --out to write to a file. Use --view to render the image inline in compatible terminals (iTerm2).
`),
		Example: strings.TrimSpace(`
  verkada cameras thumbnail --camera-id CAM123 > thumb.jpg
  verkada cameras thumbnail --camera-id CAM123 --timestamp 1736893300 --resolution hi-res --out thumb.jpg
  verkada cameras thumbnail --camera-id CAM123 --view
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := effectiveConfig(*rf)
			if err != nil {
				return err
			}

			if strings.TrimSpace(f.CameraID) == "" {
				return errors.New("--camera-id is required")
			}
			if f.Resolution == "" {
				f.Resolution = "low-res"
			}
			if f.Resolution != "low-res" && f.Resolution != "hi-res" {
				return fmt.Errorf("invalid --resolution %q (expected low-res or hi-res)", f.Resolution)
			}

			ts := f.Timestamp
			if ts == 0 {
				ts = time.Now().Unix()
			}

			reqURL, err := buildCamerasThumbnailURL(cfg.BaseURL, f.CameraID, ts, f.Resolution)
			if err != nil {
				return err
			}

			req, err := http.NewRequest("GET", reqURL, nil)
			if err != nil {
				return err
			}

			applyDefaultHeaders(req, cfg)
			if err := applyHeaderFlags(req, rf.Headers); err != nil {
				return err
			}
			applyBestEffortAuth(req, cfg)

			client := &http.Client{Timeout: f.Timeout}
			start := time.Now()
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			b, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			if looksLikeHTML(resp.Header.Get("Content-Type"), b) {
				return fmt.Errorf("received HTML instead of JPEG (check --base-url is https://api(.eu|.au).verkada.com and auth headers x-api-key / x-verkada-auth)")
			}

			// Auto-fetch API token if required/expired and retry once.
			if refreshed, err := maybeRefreshTokenOnAuthError(client, &cfg, rf, resp.StatusCode, b); err != nil {
				return err
			} else if refreshed {
				req2, err := http.NewRequest("GET", reqURL, nil)
				if err != nil {
					return err
				}
				applyDefaultHeaders(req2, cfg)
				if err := applyHeaderFlags(req2, rf.Headers); err != nil {
					return err
				}
				applyBestEffortAuth(req2, cfg)

				start2 := time.Now()
				resp2, err := client.Do(req2)
				if err != nil {
					return err
				}
				defer resp2.Body.Close()

				b2, err := io.ReadAll(resp2.Body)
				if err != nil {
					return err
				}
				if looksLikeHTML(resp2.Header.Get("Content-Type"), b2) {
					return fmt.Errorf("received HTML instead of JPEG (check --base-url is https://api(.eu|.au).verkada.com and auth headers x-api-key / x-verkada-auth)")
				}
				if rf.Debug {
					fmt.Fprintf(cmd.ErrOrStderr(), "HTTP %s %s -> %d (%s)\n", req2.Method, req2.URL.String(), resp2.StatusCode, time.Since(start2))
				}
				resp = resp2
				b = b2
			}

			if rf.Debug {
				fmt.Fprintf(cmd.ErrOrStderr(), "HTTP %s %s -> %d (%s)\n", req.Method, req.URL.String(), resp.StatusCode, time.Since(start))
			}

			// Even if the server doesn't set Content-Type reliably, this endpoint is documented as JPEG bytes.
			// If it returns JSON on error, surface it to the user.
			if resp.StatusCode >= 400 || looksLikeJSON(resp.Header.Get("Content-Type"), b) {
				// Respect global output setting for JSON/text here.
				out := cmd.OutOrStdout()
				if pretty, ok := tryPrettyJSON(b); ok {
					_, _ = out.Write(pretty)
				} else {
					_, _ = out.Write(b)
					if len(b) == 0 || b[len(b)-1] != '\n' {
						fmt.Fprintln(out)
					}
				}
				if resp.StatusCode >= 400 {
					return fmt.Errorf("request failed with status %d", resp.StatusCode)
				}
				// If it's JSON but 200, still treat as unexpected.
				return errors.New("unexpected JSON response for thumbnail endpoint")
			}

			// Write JPEG bytes to file and/or stdout.
			if f.OutPath != "" {
				if err := os.MkdirAll(filepath.Dir(f.OutPath), 0o755); err != nil && filepath.Dir(f.OutPath) != "." {
					return err
				}
				if err := os.WriteFile(f.OutPath, b, 0o644); err != nil {
					return err
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s (%d bytes)\n", f.OutPath, len(b))
			} else {
				_, _ = cmd.OutOrStdout().Write(b)
			}

			if f.View {
				// Prefer to render from the bytes we already fetched, regardless of --out.
				// iTerm2 inline images protocol: https://iterm2.com/documentation-images.html
				if err := iterm2InlineJPEG(cmd.ErrOrStderr(), b, f.CameraID, ts); err != nil {
					return err
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&f.CameraID, "camera-id", "", "Camera ID (required)")
	cmd.Flags().Int64Var(&f.Timestamp, "timestamp", 0, "Unix timestamp in seconds (default: now)")
	cmd.Flags().StringVar(&f.Resolution, "resolution", "low-res", "Thumbnail resolution: low-res|hi-res")
	cmd.Flags().StringVarP(&f.OutPath, "out", "o", "", "Write JPEG to file instead of stdout")
	cmd.Flags().BoolVar(&f.View, "view", false, "Render the image inline in terminal (iTerm2)")
	cmd.Flags().DurationVar(&f.Timeout, "timeout", 30*time.Second, "HTTP timeout")

	return cmd
}

func buildCamerasThumbnailURL(baseURL, cameraID string, ts int64, resolution string) (string, error) {
	bu, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	pu, err := url.Parse("/cameras/v1/footage/thumbnails")
	if err != nil {
		return "", err
	}
	u := bu.ResolveReference(pu)
	q := u.Query()
	q.Set("camera_id", cameraID)
	if ts != 0 {
		q.Set("timestamp", strconv.FormatInt(ts, 10))
	}
	if strings.TrimSpace(resolution) != "" {
		q.Set("resolution", resolution)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func buildCamerasDevicesURL(baseURL string) (string, error) {
	bu, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	pu, err := url.Parse("/cameras/v1/devices")
	if err != nil {
		return "", err
	}
	return bu.ResolveReference(pu).String(), nil
}

func doCamerasDevicesRequest(client *http.Client, cfg *Config, rf *rootFlags, pageToken string, pageSize int) ([]byte, string, int, error) {
	reqURL, err := buildCamerasDevicesURL(cfg.BaseURL)
	if err != nil {
		return nil, "", 0, err
	}

	u, err := url.Parse(reqURL)
	if err != nil {
		return nil, "", 0, err
	}
	q := u.Query()
	if strings.TrimSpace(pageToken) != "" {
		q.Set("page_token", pageToken)
	}
	if pageSize > 0 {
		if pageSize > 200 {
			pageSize = 200
		}
		q.Set("page_size", strconv.Itoa(pageSize))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, "", 0, err
	}

	applyDefaultHeaders(req, *cfg)
	if err := applyHeaderFlags(req, rf.Headers); err != nil {
		return nil, "", 0, err
	}
	applyBestEffortAuth(req, *cfg)

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", 0, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", 0, err
	}

	// Auto-fetch a short-lived API token if required/expired and retry once.
	if refreshed, err := maybeRefreshTokenOnAuthError(client, cfg, rf, resp.StatusCode, b); err != nil {
		return nil, "", 0, err
	} else if refreshed {
		req2, err := http.NewRequest("GET", u.String(), nil)
		if err != nil {
			return nil, "", 0, err
		}
		applyDefaultHeaders(req2, *cfg)
		if err := applyHeaderFlags(req2, rf.Headers); err != nil {
			return nil, "", 0, err
		}
		applyBestEffortAuth(req2, *cfg)
		start2 := time.Now()
		resp2, err := client.Do(req2)
		if err != nil {
			return nil, "", 0, err
		}
		defer resp2.Body.Close()
		b2, err := io.ReadAll(resp2.Body)
		if err != nil {
			return nil, "", 0, err
		}
		if rf.Debug {
			fmt.Fprintf(os.Stderr, "HTTP %s %s -> %d (%s)\n", req2.Method, req2.URL.String(), resp2.StatusCode, time.Since(start2))
		}
		return b2, resp2.Header.Get("Content-Type"), resp2.StatusCode, nil
	}

	if rf.Debug {
		fmt.Fprintf(os.Stderr, "HTTP %s %s -> %d (%s)\n", req.Method, req.URL.String(), resp.StatusCode, time.Since(start))
	}

	return b, resp.Header.Get("Content-Type"), resp.StatusCode, nil
}

func iterm2InlineJPEG(w io.Writer, jpeg []byte, cameraID string, ts int64) error {
	if len(jpeg) == 0 {
		return errors.New("empty image")
	}
	name := fmt.Sprintf("thumbnail_%s_%d.jpg", cameraID, ts)
	b64 := base64.StdEncoding.EncodeToString(jpeg)
	// Emit to stderr so stdout can still be redirected to a file cleanly.
	_, err := fmt.Fprintf(w, "\033]1337;File=name=%s;inline=1;size=%d;preserveAspectRatio=1:%s\a\n",
		base64.StdEncoding.EncodeToString([]byte(name)),
		len(jpeg),
		b64,
	)
	return err
}

func formatCameraListText(body []byte, wide bool, labels *LocalLabels) (string, error) {
	devs, err := extractDeviceArray(body)
	if err != nil {
		return "", err
	}
	if len(devs) == 0 {
		return "no cameras\n", nil
	}

	var buf bytes.Buffer
	if wide {
		fmt.Fprintf(&buf, "%-36s  %-20s  %-28s  %-18s  %-10s  %-14s  %-15s  %-17s  %-10s  %-20s\n",
			"camera_id", "label", "name", "site", "model", "serial", "local_ip", "mac", "status", "timezone")
	} else {
		fmt.Fprintf(&buf, "%-36s  %-20s  %-32s  %-20s  %-10s  %-14s  %-10s\n",
			"camera_id", "label", "name", "site", "model", "serial", "status")
	}
	for _, d := range devs {
		id := pickString(d, "camera_id", "cameraId", "cameraID", "id")
		label := ""
		if labels != nil && labels.Cameras != nil {
			label = labels.Cameras[id]
		}
		name := pickString(d, "name", "device_name", "deviceName")
		site := pickString(d, "site", "site_name", "siteName")
		model := pickString(d, "model", "device_model", "deviceModel")
		serial := pickString(d, "serial", "serial_number", "serialNumber")
		status := pickString(d, "status", "camera_status", "cameraStatus")
		localIP := pickString(d, "local_ip", "localIp")
		mac := pickString(d, "mac", "mac_address", "macAddress")
		tz := pickString(d, "timezone", "time_zone", "timeZone")

		if wide {
			fmt.Fprintf(&buf, "%-36s  %-20s  %-28s  %-18s  %-10s  %-14s  %-15s  %-17s  %-10s  %-20s\n",
				trunc(id, 36),
				trunc(label, 20),
				trunc(name, 28),
				trunc(site, 18),
				trunc(model, 10),
				trunc(serial, 14),
				trunc(localIP, 15),
				trunc(mac, 17),
				trunc(status, 10),
				trunc(tz, 20),
			)
		} else {
			fmt.Fprintf(&buf, "%-36s  %-20s  %-32s  %-20s  %-10s  %-14s  %-10s\n",
				trunc(id, 36),
				trunc(label, 20),
				trunc(name, 32),
				trunc(site, 20),
				trunc(model, 10),
				trunc(serial, 14),
				trunc(status, 10),
			)
		}
	}
	return buf.String(), nil
}

func filterCameras(cams []map[string]any, cameraID string, q string, labels *LocalLabels) []map[string]any {
	cameraID = strings.TrimSpace(cameraID)
	q = strings.ToLower(strings.TrimSpace(q))
	if cameraID == "" && q == "" {
		return cams
	}
	out := make([]map[string]any, 0, len(cams))
	for _, c := range cams {
		id := pickString(c, "camera_id", "cameraId", "cameraID", "id")
		name := pickString(c, "name", "device_name", "deviceName")
		site := pickString(c, "site", "site_name", "siteName")
		label := ""
		if labels != nil && labels.Cameras != nil {
			label = labels.Cameras[id]
		}
		if cameraID != "" && id != cameraID {
			continue
		}
		if q != "" {
			hay := strings.ToLower(strings.Join([]string{id, name, site, label}, " "))
			if !strings.Contains(hay, q) {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

func trunc(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 {
		return ""
	}
	// Keep ASCII truncation simple; IDs/names here are expected to be ASCII.
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

func extractDeviceArray(body []byte) ([]map[string]any, error) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}

	switch t := v.(type) {
	case []any:
		return coerceMapSlice(t)
	case map[string]any:
		// Common envelope keys.
		for _, k := range []string{"cameras", "devices", "data", "results"} {
			if arr, ok := t[k].([]any); ok {
				return coerceMapSlice(arr)
			}
		}
		// Fallback: if there's exactly one array value, use it.
		var only []any
		for _, vv := range t {
			if arr, ok := vv.([]any); ok {
				if only != nil {
					return nil, errors.New("ambiguous response: multiple arrays present")
				}
				only = arr
			}
		}
		if only != nil {
			return coerceMapSlice(only)
		}
		return nil, errors.New("no device array found in response")
	default:
		return nil, errors.New("unexpected JSON shape")
	}
}

func extractCamerasAndNextToken(body []byte) ([]map[string]any, string, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, "", err
	}
	var cams []map[string]any
	if arr, ok := m["cameras"].([]any); ok {
		c, err := coerceMapSlice(arr)
		if err != nil {
			return nil, "", err
		}
		cams = c
	} else {
		// Be flexible; some responses might use "devices".
		if arr, ok := m["devices"].([]any); ok {
			c, err := coerceMapSlice(arr)
			if err != nil {
				return nil, "", err
			}
			cams = c
		} else {
			return nil, "", errors.New("missing cameras array")
		}
	}
	token := pickString(m, "next_page_token", "nextPageToken", "next_page", "nextPage")
	return cams, token, nil
}

func coerceMapSlice(arr []any) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func pickString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case string:
				return t
			case fmt.Stringer:
				return t.String()
			case float64:
				// JSON numbers decode to float64; render without trailing .0 when integral.
				if t == float64(int64(t)) {
					return strconv.FormatInt(int64(t), 10)
				}
				return strconv.FormatFloat(t, 'f', -1, 64)
			case bool:
				if t {
					return "true"
				}
				return "false"
			}
		}
	}
	return ""
}
