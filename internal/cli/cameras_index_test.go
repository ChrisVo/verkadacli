package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCamerasIndex_SearchBySiteToken(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "cameras.sqlite")

	rf := rootFlags{Profile: "default"}
	cfg := Config{BaseURL: "https://api.verkada.com", OrgID: "ORG"}

	cams := []map[string]any{
		{"camera_id": "cam-1", "name": "North Door", "site": "Cathedral", "model": "D40", "serial": "S1", "status": "online"},
		{"camera_id": "cam-2", "name": "Lobby", "site": "HQ", "model": "D40", "serial": "S2", "status": "online"},
	}
	labels := map[string]string{"cam-2": "Front desk"}

	if err := rebuildCamerasIndex(dbPath, rf, cfg, cams, labels); err != nil {
		t.Fatalf("rebuildCamerasIndex: %v", err)
	}

	res, err := searchCamerasIndex(dbPath, "cameras in cathedral", 10)
	if err != nil {
		t.Fatalf("searchCamerasIndex: %v", err)
	}
	if len(res.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res.Results))
	}
	if res.Results[0].CameraID != "cam-1" {
		t.Fatalf("expected cam-1, got %q", res.Results[0].CameraID)
	}
}

func TestCamerasIndex_LabelUpdateAffectsSearch(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "cameras.sqlite")

	rf := rootFlags{Profile: "default"}
	cfg := Config{BaseURL: "https://api.verkada.com", OrgID: "ORG"}

	cams := []map[string]any{
		{"camera_id": "cam-1", "name": "Door", "site": "HQ"},
	}
	if err := rebuildCamerasIndex(dbPath, rf, cfg, cams, nil); err != nil {
		t.Fatalf("rebuildCamerasIndex: %v", err)
	}

	// No results before label.
	res, err := searchCamerasIndex(dbPath, "cathedral", 10)
	if err != nil {
		t.Fatalf("searchCamerasIndex: %v", err)
	}
	if len(res.Results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(res.Results))
	}

	l := "Cathedral - Nave"
	tryUpdateIndexLabel(dbPath, "cam-1", &l)

	res, err = searchCamerasIndex(dbPath, "cathedral", 10)
	if err != nil {
		t.Fatalf("searchCamerasIndex: %v", err)
	}
	if len(res.Results) != 1 || res.Results[0].CameraID != "cam-1" {
		t.Fatalf("expected cam-1 after label, got %+v", res.Results)
	}
}

func TestCamerasIndexStatus_NotExists(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "missing.sqlite")

	_, err := readCamerasIndexStatus(dbPath)
	if err == nil {
		t.Fatalf("expected error for missing db")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected IsNotExist, got %v", err)
	}
}
