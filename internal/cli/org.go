package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func buildCoreOrganizationURL(baseURL string) (string, error) {
	bu, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	pu, err := url.Parse("/core/v1/organization")
	if err != nil {
		return "", err
	}
	return bu.ResolveReference(pu).String(), nil
}

func parseOrgIDFromBody(body []byte) (string, bool) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return "", false
	}

	var walk func(x any) string
	walk = func(x any) string {
		m, ok := x.(map[string]any)
		if !ok {
			return ""
		}

		// Common key names we might see.
		for _, k := range []string{"org_id", "orgId", "organization_id", "organizationId", "organizationID", "id"} {
			if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}

		// Common nesting.
		for _, k := range []string{"organization", "org", "data"} {
			if child, ok := m[k]; ok {
				if s := walk(child); s != "" {
					return s
				}
			}
		}

		return ""
	}

	id := walk(v)
	if strings.TrimSpace(id) == "" {
		return "", false
	}
	return id, true
}

// ensureOrgID best-effort populates cfg.OrgID if missing by calling /core/v1/organization.
// It will also persist the org id to the selected profile when possible.
func ensureOrgID(client *http.Client, cfg *Config, rf *rootFlags) (bool, error) {
	if strings.TrimSpace(cfg.OrgID) != "" {
		return false, nil
	}

	if strings.TrimSpace(cfg.Auth.APIKey) == "" {
		// Can't auto-discover without an API key.
		return false, nil
	}

	u, err := buildCoreOrganizationURL(cfg.BaseURL)
	if err != nil {
		return false, err
	}

	doOnce := func() (int, []byte, error) {
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return 0, nil, err
		}
		applyDefaultHeaders(req, *cfg)
		if err := applyHeaderFlags(req, rf.Headers); err != nil {
			return 0, nil, err
		}
		applyBestEffortAuth(req, *cfg)

		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return resp.StatusCode, nil, err
		}
		if rf.Debug {
			fmt.Fprintf(os.Stderr, "HTTP %s %s -> %d (%s)\n", req.Method, req.URL.String(), resp.StatusCode, time.Since(start))
		}
		return resp.StatusCode, b, nil
	}

	status, b, err := doOnce()
	if err != nil {
		return false, err
	}
	if looksLikeHTML("", b) {
		return false, errors.New("received HTML from /core/v1/organization (check --base-url is https://api(.eu|.au).verkada.com and auth headers)")
	}

	// Auto-fetch API token if required/expired and retry once.
	if refreshed, err := maybeRefreshTokenOnAuthError(client, cfg, rf, status, b); err != nil {
		return false, err
	} else if refreshed {
		status, b, err = doOnce()
		if err != nil {
			return false, err
		}
	}

	if status >= 400 {
		// Provide a helpful error for common cases, but keep this best-effort.
		if msg, ok := apiErrorMessage(b); ok {
			lm := strings.ToLower(msg)
			if status == 403 && strings.Contains(lm, "insufficient permissions") {
				return false, errors.New("cannot auto-discover org id via /core/v1/organization: insufficient permissions for this API key (set --org-id or VERKADA_ORG_ID manually)")
			}
			if status == 401 {
				return false, fmt.Errorf("cannot auto-discover org id via /core/v1/organization: authentication failed (%s)", msg)
			}
		}
		return false, nil
	}

	orgID, ok := parseOrgIDFromBody(b)
	if !ok {
		return false, nil
	}

	cfg.OrgID = orgID
	_ = persistProfileOrgID(*rf, orgID) // best-effort
	return true, nil
}
