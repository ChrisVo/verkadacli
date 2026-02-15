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

type apiErrorResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	// Data is often null; keep it as raw.
	Data any `json:"data"`
}

func apiErrorMessage(body []byte) (string, bool) {
	var e apiErrorResponse
	if err := json.Unmarshal(body, &e); err != nil {
		return "", false
	}
	if strings.TrimSpace(e.Message) == "" {
		return "", false
	}
	return e.Message, true
}

func isAPITokenRequired(status int, body []byte) bool {
	if status != 400 {
		return false
	}
	msg, ok := apiErrorMessage(body)
	if !ok {
		return false
	}
	return strings.Contains(strings.ToLower(msg), "api token is required")
}

func isAPITokenExpired(status int, body []byte) bool {
	if status != 401 {
		return false
	}
	msg, ok := apiErrorMessage(body)
	if !ok {
		return false
	}
	return strings.Contains(strings.ToLower(msg), "token expired")
}

func buildTokenURL(baseURL string) (string, error) {
	bu, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	pu, err := url.Parse("/token")
	if err != nil {
		return "", err
	}
	return bu.ResolveReference(pu).String(), nil
}

func fetchAPIToken(client *http.Client, cfg Config, rf *rootFlags) (string, error) {
	if strings.TrimSpace(cfg.Auth.APIKey) == "" {
		return "", errors.New("cannot fetch API token: api key is empty")
	}
	tu, err := buildTokenURL(cfg.BaseURL)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", tu, nil)
	if err != nil {
		return "", err
	}
	applyDefaultHeaders(req, cfg)
	if err := applyHeaderFlags(req, rf.Headers); err != nil {
		return "", err
	}
	// Force API key for token endpoint.
	req.Header.Set("x-api-key", cfg.Auth.APIKey)

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if rf.Debug {
		fmt.Fprintf(os.Stderr, "HTTP %s %s -> %d (%s)\n", req.Method, req.URL.String(), resp.StatusCode, time.Since(start))
	}

	if looksLikeHTML(resp.Header.Get("Content-Type"), b) {
		return "", errors.New("received HTML from /token (base URL likely points to Command web UI, not api.*.verkada.com)")
	}

	if resp.StatusCode >= 400 {
		if pretty, ok := tryPrettyJSON(b); ok {
			return "", fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(pretty)))
		}
		return "", fmt.Errorf("token request failed with status %d", resp.StatusCode)
	}

	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.Token) == "" {
		return "", errors.New("token response missing token field")
	}
	return out.Token, nil
}

func maybeRefreshTokenOnAuthError(client *http.Client, cfg *Config, rf *rootFlags, status int, body []byte) (bool, error) {
	if !(isAPITokenRequired(status, body) || isAPITokenExpired(status, body)) {
		return false, nil
	}
	tok, err := fetchAPIToken(client, *cfg, rf)
	if err != nil {
		return false, err
	}
	cfg.Auth.Token = tok
	cfg.Auth.TokenAcquiredAt = time.Now().Unix()
	_ = persistProfileToken(*rf, cfg.Auth.Token, cfg.Auth.TokenAcquiredAt) // best-effort
	return true, nil
}

func persistProfileToken(rf rootFlags, token string, acquiredAt int64) error {
	p, err := resolveConfigPath(rf.ConfigPath)
	if err != nil {
		return err
	}
	cf, err := loadConfig(p)
	if err != nil {
		return err
	}
	normalizeConfigFile(&cf)
	profileName := firstNonEmpty(rf.Profile, envOr("VERKADA_PROFILE", ""), cf.CurrentProfile, "default")
	profile, ok := cf.Profiles[profileName]
	if !ok {
		return fmt.Errorf("profile %q not found in %s", profileName, p)
	}
	profile.Auth.Token = token
	profile.Auth.TokenAcquiredAt = acquiredAt
	cf.Profiles[profileName] = profile
	return writeConfig(p, cf)
}
