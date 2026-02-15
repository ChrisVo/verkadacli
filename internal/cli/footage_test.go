package cli

import (
	"net/url"
	"strings"
	"testing"
)

func TestBuildFootageStreamM3U8URL_History(t *testing.T) {
	u, err := buildFootageStreamM3U8URL(
		"https://api.verkada.com",
		"ORG1",
		"CAM1",
		"JWT1",
		1739570400,
		1739570700,
		"high_res",
		"hevc",
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasPrefix(u, "https://api.verkada.com/stream/cameras/v1/footage/stream/stream.m3u8?") {
		t.Fatalf("unexpected url: %s", u)
	}
	if !strings.Contains(u, "org_id=ORG1") || !strings.Contains(u, "camera_id=CAM1") || !strings.Contains(u, "jwt=JWT1") {
		t.Fatalf("missing required query params: %s", u)
	}
	if !strings.Contains(u, "start_time=1739570400") || !strings.Contains(u, "end_time=1739570700") {
		t.Fatalf("missing time params: %s", u)
	}
	if !strings.Contains(u, "resolution=high_res") {
		t.Fatalf("missing resolution: %s", u)
	}
}

func TestRewriteM3U8_RewritesRelativeAndAddsQuery(t *testing.T) {
	playlistURL, _ := url.Parse("https://api.verkada.com/stream/cameras/v1/footage/stream/stream.m3u8?org_id=ORG&camera_id=CAM&jwt=JWT&start_time=1&end_time=2&type=stream")
	required := playlistURL.Query()
	in := strings.Join([]string{
		"#EXTM3U",
		`#EXT-X-KEY:METHOD=AES-128,URI="enc.key"`,
		"#EXT-X-MAP:URI=\"init.mp4\"",
		"#EXTINF:2.0,",
		"seg1.m4s",
		"#EXTINF:2.0,",
		"https://cdn.example.com/seg2.m4s",
		"",
	}, "\n")

	out, err := rewriteM3U8([]byte(in), playlistURL, required)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	s := string(out)

	// Relative tag URIs should be rewritten to absolute URLs with required query params.
	if !strings.Contains(s, `URI="https://api.verkada.com/stream/cameras/v1/footage/stream/enc.key?`) {
		t.Fatalf("expected rewritten key uri, got:\n%s", s)
	}
	if !strings.Contains(s, `URI="https://api.verkada.com/stream/cameras/v1/footage/stream/init.mp4?`) {
		t.Fatalf("expected rewritten map uri, got:\n%s", s)
	}
	if !strings.Contains(s, "https://api.verkada.com/stream/cameras/v1/footage/stream/seg1.m4s?") {
		t.Fatalf("expected rewritten segment uri, got:\n%s", s)
	}

	// Absolute URIs should remain absolute.
	if !strings.Contains(s, "https://cdn.example.com/seg2.m4s") {
		t.Fatalf("expected preserved absolute uri, got:\n%s", s)
	}

	// Ensure required query params were added to rewritten relative URIs.
	for _, k := range []string{"org_id=ORG", "camera_id=CAM", "jwt=JWT"} {
		if !strings.Contains(s, k) {
			t.Fatalf("expected required query param %q present, got:\n%s", k, s)
		}
	}
}
