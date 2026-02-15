package cli

import "testing"

func TestBuildRequestURL(t *testing.T) {
	u, err := buildRequestURL("https://api.example.com/", "", "/v1/foo", []string{"a=b", "a=c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// url.Values.Encode sorts by key; for identical keys, insertion order is preserved in Go.
	if u != "https://api.example.com/v1/foo?a=b&a=c" {
		t.Fatalf("unexpected url: %s", u)
	}
}
