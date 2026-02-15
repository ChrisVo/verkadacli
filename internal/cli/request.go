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

	"github.com/spf13/cobra"
)

type requestFlags struct {
	Method      string
	Path        string
	URL         string
	Query       []string
	Body        string
	ShowHeaders bool
	Timeout     time.Duration
}

func NewRequestCmd(rf *rootFlags) *cobra.Command {
	var f requestFlags

	cmd := &cobra.Command{
		Use:   "request",
		Short: "Make a raw HTTP request (useful until typed commands exist)",
		Example: strings.TrimSpace(`
  verkada config init
  verkada request --method GET --path /v1/cameras
  verkada request -H 'x-api-key: ...' --method GET --url https://api.verkada.com/v1/cameras
  verkada request --method POST --path /v1/foo --body @payload.json
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := effectiveConfig(*rf)
			if err != nil {
				return err
			}

			reqURL, err := buildRequestURL(cfg.BaseURL, f.URL, f.Path, f.Query)
			if err != nil {
				return err
			}

			var bodyBytes []byte
			if f.Body != "" {
				b, err := readBodyArg(f.Body)
				if err != nil && !errors.Is(err, errNoBody) {
					return err
				}
				bodyBytes = b
			}

			client := &http.Client{Timeout: f.Timeout}
			doOnce := func() (*http.Request, *http.Response, []byte, time.Duration, error) {
				var bodyReader io.Reader
				if bodyBytes != nil {
					bodyReader = bytes.NewReader(bodyBytes)
				}

				req, err := http.NewRequest(strings.ToUpper(f.Method), reqURL, bodyReader)
				if err != nil {
					return nil, nil, nil, 0, err
				}

				applyDefaultHeaders(req, cfg)
				if err := applyHeaderFlags(req, rf.Headers); err != nil {
					return nil, nil, nil, 0, err
				}
				applyBestEffortAuth(req, cfg)

				start := time.Now()
				resp, err := client.Do(req)
				if err != nil {
					return req, nil, nil, 0, err
				}
				defer resp.Body.Close()

				b, err := io.ReadAll(resp.Body)
				if err != nil {
					return req, resp, nil, time.Since(start), err
				}
				return req, resp, b, time.Since(start), nil
			}

			req, resp, b, dur, err := doOnce()
			if err != nil {
				return err
			}

			// Auto-fetch API token if required/expired and retry once.
			if refreshed, err := maybeRefreshTokenOnAuthError(client, &cfg, rf, resp.StatusCode, b); err != nil {
				return err
			} else if refreshed {
				req, resp, b, dur, err = doOnce()
				if err != nil {
					return err
				}
			}

			if rf.Debug {
				fmt.Fprintf(cmd.ErrOrStderr(), "HTTP %s %s -> %d (%s)\n", req.Method, req.URL.String(), resp.StatusCode, dur)
			}

			if looksLikeHTML(resp.Header.Get("Content-Type"), b) {
				return fmt.Errorf("received HTML response (check --base-url is https://api(.eu|.au).verkada.com and auth headers x-api-key / x-verkada-auth)")
			}

			if f.ShowHeaders {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n", resp.Status)
				for k, vals := range resp.Header {
					for _, v := range vals {
						fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", k, v)
					}
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}

			out := cmd.OutOrStdout()
			if rf.Output == "json" || looksLikeJSON(resp.Header.Get("Content-Type"), b) {
				if pretty, ok := tryPrettyJSON(b); ok {
					_, _ = out.Write(pretty)
					if len(pretty) == 0 || pretty[len(pretty)-1] != '\n' {
						fmt.Fprintln(out)
					}
				} else {
					_, _ = out.Write(b)
					if len(b) == 0 || b[len(b)-1] != '\n' {
						fmt.Fprintln(out)
					}
				}
			} else {
				_, _ = out.Write(b)
				if len(b) == 0 || b[len(b)-1] != '\n' {
					fmt.Fprintln(out)
				}
			}

			if resp.StatusCode >= 400 {
				return fmt.Errorf("request failed with status %d", resp.StatusCode)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&f.Method, "method", "GET", "HTTP method")
	cmd.Flags().StringVar(&f.Path, "path", "", "Path to request, joined with base URL (ignored if --url is set)")
	cmd.Flags().StringVar(&f.URL, "url", "", "Full request URL (overrides base URL + path)")
	cmd.Flags().StringArrayVar(&f.Query, "query", nil, "Query param (repeatable), e.g. --query a=b")
	cmd.Flags().StringVar(&f.Body, "body", "", "Request body; prefix with @ to read from file (e.g. @payload.json)")
	cmd.Flags().BoolVar(&f.ShowHeaders, "show-headers", false, "Print response status line and headers")
	cmd.Flags().DurationVar(&f.Timeout, "timeout", 30*time.Second, "HTTP timeout")

	return cmd
}

func buildRequestURL(baseURL, fullURL, path string, query []string) (string, error) {
	var u *url.URL
	var err error

	if fullURL != "" {
		u, err = url.Parse(fullURL)
		if err != nil {
			return "", err
		}
	} else {
		if path == "" {
			return "", errors.New("either --url or --path is required")
		}
		bu, err := url.Parse(baseURL)
		if err != nil {
			return "", err
		}
		pu, err := url.Parse(path)
		if err != nil {
			return "", err
		}
		u = bu.ResolveReference(pu)
	}

	q := u.Query()
	for _, kv := range query {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return "", fmt.Errorf("invalid --query %q (expected k=v)", kv)
		}
		q.Add(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func applyDefaultHeaders(req *http.Request, cfg Config) {
	for k, v := range cfg.Headers {
		if strings.TrimSpace(k) == "" {
			continue
		}
		req.Header.Set(k, v)
	}

	// Common default when a body is present; users can override with -H.
	if req.Body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
}

func applyHeaderFlags(req *http.Request, headers []string) error {
	for _, h := range headers {
		k, v, ok := strings.Cut(h, ":")
		if !ok {
			return fmt.Errorf("invalid header %q (expected 'Key: Value')", h)
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" {
			return fmt.Errorf("invalid header %q (empty key)", h)
		}
		req.Header.Add(k, v)
	}
	return nil
}

func applyBestEffortAuth(req *http.Request, cfg Config) {
	// If user already set auth via -H or config headers, don't override.
	// (But we still allow filling in missing Verkada-specific auth headers.)

	// If an API key is present and no known header is set, use x-api-key.
	if cfg.Auth.APIKey != "" && req.Header.Get("x-api-key") == "" && req.Header.Get("X-Api-Key") == "" {
		req.Header.Set("x-api-key", cfg.Auth.APIKey)
	}

	// Verkada API token is carried in x-verkada-auth (per OpenAPI).
	if cfg.Auth.Token != "" && req.Header.Get("x-verkada-auth") == "" && req.Header.Get("X-Verkada-Auth") == "" {
		req.Header.Set("x-verkada-auth", cfg.Auth.Token)
	}

	// Best-effort fallback: if caller set neither Verkada token header nor Authorization,
	// use Bearer for compatibility with other endpoints/tools.
	if cfg.Auth.Token != "" && req.Header.Get("Authorization") == "" && req.Header.Get("x-verkada-auth") != "" {
		// Don't also set Authorization; prefer the Verkada header when present.
		return
	}
	if cfg.Auth.Token != "" && req.Header.Get("Authorization") == "" && req.Header.Get("x-verkada-auth") == "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Auth.Token)
	}
}

func looksLikeHTML(contentType string, body []byte) bool {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml") {
		return true
	}
	trim := bytes.TrimSpace(body)
	if len(trim) == 0 {
		return false
	}
	// Cheap sniff to catch Command web app HTML.
	s := strings.ToLower(string(trim))
	return strings.HasPrefix(s, "<!doctype html") || strings.HasPrefix(s, "<html") || strings.Contains(s, "<title>verkada</title>")
}

func looksLikeJSON(contentType string, body []byte) bool {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "application/json") || strings.Contains(ct, "+json") {
		return true
	}
	trim := bytes.TrimSpace(body)
	return len(trim) > 0 && (trim[0] == '{' || trim[0] == '[')
}

func tryPrettyJSON(b []byte) ([]byte, bool) {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, false
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, false
	}
	out = append(out, '\n')
	return out, true
}
