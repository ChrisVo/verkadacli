package cli

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

// camerasIndexSchemaVersion is used to detect incompatible on-disk schema changes.
const camerasIndexSchemaVersion = 1

func newCamerasIndexCmd(rf *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Manage the local camera search index",
	}
	cmd.AddCommand(newCamerasIndexBuildCmd(rf))
	cmd.AddCommand(newCamerasIndexStatusCmd(rf))
	return cmd
}

func newCamerasIndexBuildCmd(rf *rootFlags) *cobra.Command {
	var timeout time.Duration
	var pageSize int

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build (or rebuild) the local SQLite camera index for fast search",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := effectiveConfig(*rf)
			if err != nil {
				return err
			}

			idxPath, err := camerasIndexPath(*rf, cfg)
			if err != nil {
				return err
			}

			client := &http.Client{Timeout: timeout}
			cams, err := fetchAllCameras(client, &cfg, rf, pageSize)
			if err != nil {
				return err
			}

			labels := map[string]string{}
			if cfg.Labels != nil && cfg.Labels.Cameras != nil {
				for k, v := range cfg.Labels.Cameras {
					labels[k] = v
				}
			}

			if err := rebuildCamerasIndex(idxPath, *rf, cfg, cams, labels); err != nil {
				return err
			}

			// Human hint only; stdout stays clean (especially for --output json elsewhere).
			fmt.Fprintf(cmd.ErrOrStderr(), "indexed %d cameras at %s\n", len(cams), idxPath)
			return nil
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "HTTP timeout")
	cmd.Flags().IntVar(&pageSize, "page-size", 200, "Page size (default 200, max 200)")
	return cmd
}

func newCamerasIndexStatusCmd(rf *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show index status for the selected profile/org",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := effectiveConfig(*rf)
			if err != nil {
				return err
			}

			idxPath, err := camerasIndexPath(*rf, cfg)
			if err != nil {
				return err
			}

			s, err := readCamerasIndexStatus(idxPath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					if rf.Output == "json" {
						blob, _ := json.MarshalIndent(map[string]any{
							"exists": false,
							"path":   idxPath,
						}, "", "  ")
						blob = append(blob, '\n')
						_, _ = cmd.OutOrStdout().Write(blob)
						return nil
					}
					return fmt.Errorf("index not found at %s (run: verkcli cameras index build)", idxPath)
				}
				return err
			}

			if rf.Output == "json" {
				blob, err := json.MarshalIndent(s, "", "  ")
				if err != nil {
					return err
				}
				blob = append(blob, '\n')
				_, _ = cmd.OutOrStdout().Write(blob)
				return nil
			}

			// Text output intentionally compact for humans.
			fmt.Fprintf(cmd.OutOrStdout(), "path: %s\nexists: %v\nbuilt_at: %s\ncamera_count: %d\nschema_version: %d\nbase_url: %s\norg_id: %s\nprofile: %s\n",
				s.Path, s.Exists, unixToRFC3339(s.BuiltAt), s.CameraCount, s.SchemaVersion, s.BaseURL, s.OrgID, s.Profile)
			return nil
		},
	}
	return cmd
}

func newCamerasSearchCmd(rf *rootFlags) *cobra.Command {
	var limit int
	var wide bool

	cmd := &cobra.Command{
		Use:   "search QUERY",
		Short: "Search cameras using the local index (FTS5)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.TrimSpace(args[0])
			if query == "" {
				return errors.New("query is empty")
			}

			cfg, err := effectiveConfig(*rf)
			if err != nil {
				return err
			}

			idxPath, err := camerasIndexPath(*rf, cfg)
			if err != nil {
				return err
			}

			res, err := searchCamerasIndex(idxPath, query, limit)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("index not found at %s (run: verkcli cameras index build)", idxPath)
				}
				return err
			}

			if rf.Output == "json" {
				blob, err := json.MarshalIndent(map[string]any{
					"query":        query,
					"index_path":   idxPath,
					"result_count": len(res.Results),
					"results":      res.Results,
				}, "", "  ")
				if err != nil {
					return err
				}
				blob = append(blob, '\n')
				_, _ = cmd.OutOrStdout().Write(blob)
				return nil
			}

			// Reuse the existing camera list formatter for consistent output.
			cams := make([]map[string]any, 0, len(res.Results))
			for _, r := range res.Results {
				cams = append(cams, r.Camera)
			}
			blob, err := json.Marshal(map[string]any{"cameras": cams})
			if err != nil {
				return err
			}
			s, err := formatCameraListText(blob, wide, cfg.Labels)
			if err != nil {
				// Fallback to JSON.
				pretty, _ := json.MarshalIndent(map[string]any{"cameras": cams}, "", "  ")
				pretty = append(pretty, '\n')
				_, _ = cmd.OutOrStdout().Write(pretty)
				return nil
			}
			fmt.Fprint(cmd.OutOrStdout(), s)
			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "Max results to return")
	cmd.Flags().BoolVar(&wide, "wide", false, "Include more columns in text output")
	return cmd
}

type camerasIndexStatus struct {
	Exists        bool   `json:"exists"`
	Path          string `json:"path"`
	SchemaVersion int    `json:"schema_version"`
	BuiltAt       int64  `json:"built_at"`
	CameraCount   int    `json:"camera_count"`
	BaseURL       string `json:"base_url"`
	OrgID         string `json:"org_id"`
	Profile       string `json:"profile"`
}

type camerasIndexSearchResult struct {
	CameraID string         `json:"camera_id"`
	Rank     float64        `json:"rank"`   // lower is better (bm25)
	Camera   map[string]any `json:"camera"` // raw-ish camera object (from API), used by get/thumbnail flows
}

type camerasIndexSearchResponse struct {
	Results []camerasIndexSearchResult
}

func unixToRFC3339(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}

func camerasIndexPath(rf rootFlags, cfg Config) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	// Partition by base_url host + org_id + profile to avoid cross-contamination.
	host := "unknown"
	if strings.TrimSpace(cfg.BaseURL) != "" {
		u, err := url.Parse(cfg.BaseURL)
		if err == nil && strings.TrimSpace(u.Host) != "" {
			host = u.Host
		}
	}
	host = sanitizePathComponent(host)
	org := sanitizePathComponent(firstNonEmpty(cfg.OrgID, "no-org"))

	profile := selectedProfileNameFromConfig(rf)
	profile = sanitizePathComponent(firstNonEmpty(profile, "default"))

	dir := filepath.Join(cacheDir, "verkcli", "index", host, org, profile)
	return filepath.Join(dir, "cameras.sqlite"), nil
}

func sanitizePathComponent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	out = strings.Trim(out, "_")
	if out == "" {
		return "unknown"
	}
	return out
}

func selectedProfileNameFromConfig(rf rootFlags) string {
	// Mirror selection semantics used by other commands: flag/env > config current_profile > default.
	p, err := resolveConfigPath(rf.ConfigPath)
	if err != nil {
		return firstNonEmpty(rf.Profile, envFirst("", "VERKCLI_PROFILE", "VERKADA_PROFILE"), "default")
	}
	cf, err := loadConfig(p)
	if err != nil {
		return firstNonEmpty(rf.Profile, envFirst("", "VERKCLI_PROFILE", "VERKADA_PROFILE"), "default")
	}
	return firstNonEmpty(rf.Profile, envFirst("", "VERKCLI_PROFILE", "VERKADA_PROFILE"), cf.CurrentProfile, "default")
}

func fetchAllCameras(client *http.Client, cfg *Config, rf *rootFlags, pageSize int) ([]map[string]any, error) {
	if pageSize <= 0 {
		pageSize = 200
	}
	if pageSize > 200 {
		pageSize = 200
	}

	agg := make([]map[string]any, 0, 256)
	next := ""
	for {
		b, _, status, err := doCamerasDevicesRequest(client, cfg, rf, next, pageSize)
		if err != nil {
			return nil, err
		}
		if looksLikeHTML("", b) {
			return nil, fmt.Errorf("received HTML instead of camera JSON (check --base-url is https://api(.eu|.au).verkada.com and auth headers x-api-key / x-verkada-auth)")
		}
		if status >= 400 {
			return nil, fmt.Errorf("request failed with status %d", status)
		}

		cams, token, err := extractCamerasAndNextToken(b)
		if err != nil {
			return nil, err
		}
		agg = append(agg, cams...)
		if strings.TrimSpace(token) == "" {
			break
		}
		next = token
	}

	// Keep deterministic ordering for stable indexes.
	sort.Slice(agg, func(i, j int) bool {
		return pickString(agg[i], "camera_id", "cameraId", "cameraID", "id") < pickString(agg[j], "camera_id", "cameraId", "cameraID", "id")
	})
	return agg, nil
}

func rebuildCamerasIndex(path string, rf rootFlags, cfg Config, cams []map[string]any, labels map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := initCamerasIndexSchema(db); err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Unix()

	if _, err := tx.Exec(`DELETE FROM cameras`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM labels`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM cameras_fts`); err != nil {
		return err
	}

	if _, err := tx.Exec(`INSERT INTO meta(key,value) VALUES('schema_version', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, strconv.Itoa(camerasIndexSchemaVersion)); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO meta(key,value) VALUES('built_at', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, strconv.FormatInt(now, 10)); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO meta(key,value) VALUES('base_url', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, cfg.BaseURL); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO meta(key,value) VALUES('org_id', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, cfg.OrgID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO meta(key,value) VALUES('profile', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, selectedProfileNameFromConfig(rf)); err != nil {
		return err
	}

	cStmt, err := tx.Prepare(`
		INSERT INTO cameras(camera_id,name,site,model,serial,status,timezone,updated_at,raw_json)
		VALUES(?,?,?,?,?,?,?,?,?)
	`)
	if err != nil {
		return err
	}
	defer cStmt.Close()

	lStmt, err := tx.Prepare(`
		INSERT INTO labels(camera_id,label,updated_at)
		VALUES(?,?,?)
	`)
	if err != nil {
		return err
	}
	defer lStmt.Close()

	fStmt, err := tx.Prepare(`
		INSERT INTO cameras_fts(camera_id,name,site,label,model,serial,status,timezone)
		VALUES(?,?,?,?,?,?,?,?)
	`)
	if err != nil {
		return err
	}
	defer fStmt.Close()

	for _, c := range cams {
		id := pickString(c, "camera_id", "cameraId", "cameraID", "id")
		if strings.TrimSpace(id) == "" {
			continue
		}
		name := pickString(c, "name", "device_name", "deviceName")
		site := pickString(c, "site", "site_name", "siteName")
		model := pickString(c, "model", "device_model", "deviceModel")
		serial := pickString(c, "serial", "serial_number", "serialNumber")
		status := pickString(c, "status", "camera_status", "cameraStatus")
		tz := pickString(c, "timezone", "time_zone", "timeZone")

		raw, err := json.Marshal(c)
		if err != nil {
			return err
		}

		if _, err := cStmt.Exec(id, name, site, model, serial, status, tz, now, string(raw)); err != nil {
			return err
		}

		label := strings.TrimSpace(labels[id])
		if label != "" {
			if _, err := lStmt.Exec(id, label, now); err != nil {
				return err
			}
		}

		if _, err := fStmt.Exec(id, name, site, label, model, serial, status, tz); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func initCamerasIndexSchema(db *sql.DB) error {
	// Pragmas are best-effort; ignore errors on older sqlite implementations.
	_, _ = db.Exec(`PRAGMA journal_mode=WAL`)
	_, _ = db.Exec(`PRAGMA synchronous=NORMAL`)

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS cameras (
			camera_id TEXT PRIMARY KEY,
			name TEXT,
			site TEXT,
			model TEXT,
			serial TEXT,
			status TEXT,
			timezone TEXT,
			updated_at INTEGER,
			raw_json TEXT
		)
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS labels (
			camera_id TEXT PRIMARY KEY,
			label TEXT,
			updated_at INTEGER
		)
	`); err != nil {
		return err
	}
	// Contentless FTS: we manage inserts/deletes directly.
	if _, err := db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS cameras_fts USING fts5(
			camera_id UNINDEXED,
			name,
			site,
			label,
			model,
			serial,
			status,
			timezone,
			tokenize = 'unicode61'
		)
	`); err != nil {
		return err
	}
	return nil
}

func readCamerasIndexStatus(path string) (camerasIndexStatus, error) {
	var s camerasIndexStatus
	s.Path = path

	if _, err := os.Stat(path); err != nil {
		return s, err
	}
	s.Exists = true

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return s, err
	}
	defer db.Close()

	if err := initCamerasIndexSchema(db); err != nil {
		return s, err
	}

	getMeta := func(key string) string {
		var v string
		_ = db.QueryRow(`SELECT value FROM meta WHERE key=?`, key).Scan(&v)
		return v
	}
	s.SchemaVersion, _ = strconv.Atoi(getMeta("schema_version"))
	s.BuiltAt, _ = strconv.ParseInt(getMeta("built_at"), 10, 64)
	s.BaseURL = getMeta("base_url")
	s.OrgID = getMeta("org_id")
	s.Profile = getMeta("profile")

	_ = db.QueryRow(`SELECT COUNT(1) FROM cameras`).Scan(&s.CameraCount)
	return s, nil
}

func searchCamerasIndex(path string, query string, limit int) (camerasIndexSearchResponse, error) {
	var out camerasIndexSearchResponse

	if _, err := os.Stat(path); err != nil {
		return out, err
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 500 {
		limit = 500
	}

	fts, err := buildFTSQuery(query)
	if err != nil {
		return out, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return out, err
	}
	defer db.Close()

	if err := initCamerasIndexSchema(db); err != nil {
		return out, err
	}

	rows, err := db.Query(`
		SELECT c.raw_json, cameras_fts.camera_id, bm25(cameras_fts) AS rank
		FROM cameras_fts
		JOIN cameras c ON c.camera_id = cameras_fts.camera_id
		WHERE cameras_fts MATCH ?
		ORDER BY rank ASC
		LIMIT ?
	`, fts, limit)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	for rows.Next() {
		var raw string
		var cameraID string
		var rank float64
		if err := rows.Scan(&raw, &cameraID, &rank); err != nil {
			return out, err
		}
		var cam map[string]any
		if err := json.Unmarshal([]byte(raw), &cam); err != nil {
			// If we can't decode a row, skip it rather than failing the whole search.
			continue
		}
		out.Results = append(out.Results, camerasIndexSearchResult{
			CameraID: cameraID,
			Rank:     rank,
			Camera:   cam,
		})
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return out, nil
}

var camerasSearchStopwords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "at": {}, "by": {}, "for": {}, "from": {},
	"in": {}, "into": {}, "is": {}, "near": {}, "of": {}, "on": {}, "or": {}, "the": {}, "to": {}, "with": {},
	"camera": {}, "cameras": {},
}

func buildFTSQuery(q string) (string, error) {
	toks := tokenizeQuery(q)
	keep := toks[:0]
	for _, t := range toks {
		if _, ok := camerasSearchStopwords[t]; ok {
			continue
		}
		keep = append(keep, t)
	}
	if len(keep) == 0 {
		// Fall back to original tokens so "the" doesn't produce an empty query.
		keep = toks
	}
	if len(keep) == 0 {
		return "", errors.New("query has no searchable tokens")
	}

	terms := make([]string, 0, len(keep))
	for _, t := range keep {
		// Conservative: only allow ASCII word-ish chars into prefix terms.
		if t == "" {
			continue
		}
		terms = append(terms, t+"*")
	}
	if len(terms) == 0 {
		return "", errors.New("query has no searchable tokens")
	}
	return strings.Join(terms, " AND "), nil
}

func tokenizeQuery(q string) []string {
	q = strings.ToLower(q)
	var b strings.Builder
	b.Grow(len(q))
	for _, r := range q {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	parts := strings.Fields(b.String())
	return parts
}

// tryUpdateIndexLabel best-effort updates the on-disk index when labels change.
// It must never break normal label operations.
func tryUpdateIndexLabel(idxPath string, cameraID string, label *string) {
	if strings.TrimSpace(cameraID) == "" {
		return
	}
	if _, err := os.Stat(idxPath); err != nil {
		return
	}
	db, err := sql.Open("sqlite", idxPath)
	if err != nil {
		return
	}
	defer db.Close()
	if err := initCamerasIndexSchema(db); err != nil {
		return
	}

	tx, err := db.Begin()
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Unix()
	if label == nil || strings.TrimSpace(*label) == "" {
		_, _ = tx.Exec(`DELETE FROM labels WHERE camera_id=?`, cameraID)
	} else {
		_, _ = tx.Exec(`INSERT INTO labels(camera_id,label,updated_at) VALUES(?,?,?) ON CONFLICT(camera_id) DO UPDATE SET label=excluded.label, updated_at=excluded.updated_at`, cameraID, strings.TrimSpace(*label), now)
	}

	// Refresh the FTS row for this camera by deleting/re-inserting from cameras + labels.
	_, _ = tx.Exec(`DELETE FROM cameras_fts WHERE camera_id=?`, cameraID)
	var newLabel string
	if label != nil {
		newLabel = strings.TrimSpace(*label)
	}
	_, _ = tx.Exec(`
		INSERT INTO cameras_fts(camera_id,name,site,label,model,serial,status,timezone)
		SELECT camera_id,name,site,?,model,serial,status,timezone
		FROM cameras
		WHERE camera_id=?
	`, newLabel, cameraID)

	_ = tx.Commit()
}
