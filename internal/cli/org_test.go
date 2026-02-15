package cli

import "testing"

func TestParseOrgIDFromBody_TopLevel(t *testing.T) {
	body := []byte(`{"org_id":"11111111-2222-3333-4444-555555555555"}`)
	got, ok := parseOrgIDFromBody(body)
	if !ok {
		t.Fatalf("expected ok")
	}
	if got != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("got %q", got)
	}
}

func TestParseOrgIDFromBody_Nested(t *testing.T) {
	body := []byte(`{"organization":{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}}`)
	got, ok := parseOrgIDFromBody(body)
	if !ok {
		t.Fatalf("expected ok")
	}
	if got != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Fatalf("got %q", got)
	}
}
