package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Build-time OAuth client credentials. Set via -ldflags -X main.<var>=<value>.
// client IDs are safe to embed in binaries (they identify the app, not a secret).
// bitbucketClientSecret is needed only for confidential consumers; omit for public ones.
var (
	githubClientID        string
	bitbucketClientID     string
	bitbucketClientSecret string
	gitlabClientID        string
)

// OAuth event names emitted to the frontend.
const (
	oauthEventDeviceCode = "oauth:device_code" // DeviceCodePayload
	oauthEventWaiting    = "oauth:waiting"      // string message
	oauthEventComplete   = "oauth:complete"     // empty string
	oauthEventError      = "oauth:error"        // string error message
)

// DeviceCodePayload is sent to the frontend when the device code is ready.
type DeviceCodePayload struct {
	UserCode        string `json:"userCode"`
	VerificationURL string `json:"verificationURL"`
}

// StartOAuth begins the OAuth flow for the given provider in a background goroutine.
// Provider values: "github", "bitbucket", "gitlab".
// Events emitted: oauth:device_code, oauth:waiting, oauth:complete, oauth:error.
func (a *App) StartOAuth(provider string) {
	go func() {
		switch provider {
		case "github":
			a.runGitHubDeviceFlow()
		case "bitbucket":
			a.runBitbucketPKCE()
		case "gitlab":
			a.runGitLabDeviceFlow()
		default:
			wailsruntime.EventsEmit(a.ctx, oauthEventError, "Unknown provider: "+provider)
		}
	}()
}

// ── GitHub Device Authorization Flow ──────────────────────────────────────────

func (a *App) runGitHubDeviceFlow() {
	if githubClientID == "" {
		wailsruntime.EventsEmit(a.ctx, oauthEventError,
			"GitHub OAuth is not configured in this build. Contact the Minions team.")
		return
	}

	// 1. Request device + user codes.
	req, _ := http.NewRequest("POST", "https://github.com/login/device/code",
		strings.NewReader(url.Values{
			"client_id": {githubClientID},
			"scope":     {"repo workflow"},
		}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		wailsruntime.EventsEmit(a.ctx, oauthEventError, "GitHub request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	var dc struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
		Error           string `json:"error"`
		ErrorDesc       string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dc); err != nil {
		wailsruntime.EventsEmit(a.ctx, oauthEventError, "Unexpected response from GitHub.")
		return
	}
	if dc.Error != "" {
		wailsruntime.EventsEmit(a.ctx, oauthEventError, "GitHub: "+dc.ErrorDesc)
		return
	}

	interval := dc.Interval
	if interval == 0 {
		interval = 5
	}

	// 2. Show the user code and open the browser.
	wailsruntime.EventsEmit(a.ctx, oauthEventDeviceCode, DeviceCodePayload{
		UserCode:        dc.UserCode,
		VerificationURL: dc.VerificationURI,
	})
	_ = openBrowser(dc.VerificationURI)

	// 3. Poll until the user authorizes or the code expires.
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(time.Duration(interval) * time.Second)

		token, slowDown, errMsg := pollGitHubToken(githubClientID, dc.DeviceCode)
		if slowDown {
			interval += 5
			continue
		}
		if errMsg != "" {
			wailsruntime.EventsEmit(a.ctx, oauthEventError, errMsg)
			return
		}
		if token != "" {
			a.gitHost = "github.com"
			a.gitUsername = "oauth2"
			a.gitToken = token
			wailsruntime.EventsEmit(a.ctx, oauthEventComplete, "")
			return
		}
	}

	wailsruntime.EventsEmit(a.ctx, oauthEventError, "Code expired. Please try again.")
}

// pollGitHubToken polls the GitHub token endpoint once.
// Returns (token, slowDown, errorMessage). Empty token + no error = still pending.
func pollGitHubToken(clientID, deviceCode string) (token string, slowDown bool, errMsg string) {
	req, _ := http.NewRequest("POST", "https://github.com/login/oauth/access_token",
		strings.NewReader(url.Values{
			"client_id":   {clientID},
			"device_code": {deviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false, ""
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", false, ""
	}

	if result.AccessToken != "" {
		return result.AccessToken, false, ""
	}

	switch result.Error {
	case "", "authorization_pending":
		return "", false, ""
	case "slow_down":
		return "", true, ""
	case "expired_token":
		return "", false, "Code expired. Please try again."
	case "access_denied":
		return "", false, "Authorization was denied."
	default:
		return "", false, "GitHub: " + result.ErrorDesc
	}
}

// ── Bitbucket OAuth 2.0 — Authorization Code + PKCE + loopback redirect ───────

func (a *App) runBitbucketPKCE() {
	if bitbucketClientID == "" {
		wailsruntime.EventsEmit(a.ctx, oauthEventError,
			"Bitbucket OAuth is not configured in this build. Contact the Minions team.")
		return
	}

	verifier, challenge, err := newPKCEPair()
	if err != nil {
		wailsruntime.EventsEmit(a.ctx, oauthEventError, "PKCE setup failed: "+err.Error())
		return
	}

	// Bind a random local port for the redirect callback.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		wailsruntime.EventsEmit(a.ctx, oauthEventError, "Could not start local callback server: "+err.Error())
		return
	}
	port := ln.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)

	codeCh := make(chan string, 1)
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			code := r.URL.Query().Get("code")
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintln(w, `<!doctype html><html><body style="font-family:sans-serif;padding:40px;background:#0f0f17;color:#e2e8f0">
<h2 style="color:#34d399">&#x2713; Authorization complete!</h2>
<p>You can close this tab and return to the installer.</p></body></html>`)
			if code != "" {
				codeCh <- code
			}
		}),
	}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	authURL := "https://bitbucket.org/site/oauth2/authorize?" + url.Values{
		"client_id":             {bitbucketClientID},
		"response_type":         {"code"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"redirect_uri":          {redirectURI},
	}.Encode()

	_ = openBrowser(authURL)
	wailsruntime.EventsEmit(a.ctx, oauthEventWaiting, "Complete authorization in your browser…")

	select {
	case code := <-codeCh:
		tok, err := exchangeBitbucketCode(code, verifier, redirectURI)
		if err != nil {
			wailsruntime.EventsEmit(a.ctx, oauthEventError, err.Error())
			return
		}
		a.gitHost = "bitbucket.org"
		a.gitUsername = "x-token-auth"
		a.gitToken = tok
		wailsruntime.EventsEmit(a.ctx, oauthEventComplete, "")
	case <-time.After(10 * time.Minute):
		wailsruntime.EventsEmit(a.ctx, oauthEventError, "Authorization timed out. Please try again.")
	}
}

func exchangeBitbucketCode(code, verifier, redirectURI string) (string, error) {
	params := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
		"client_id":     {bitbucketClientID},
	}
	if bitbucketClientSecret != "" {
		params.Set("client_secret", bitbucketClientSecret)
	}

	resp, err := http.PostForm("https://bitbucket.org/site/oauth2/access_token", params)
	if err != nil {
		return "", fmt.Errorf("Bitbucket token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("unexpected Bitbucket response: %s", body)
	}
	if result.Error != "" {
		return "", fmt.Errorf("Bitbucket: %s — %s", result.Error, result.ErrorDesc)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("Bitbucket returned no access token")
	}
	return result.AccessToken, nil
}

// ── GitLab Device Authorization Flow ─────────────────────────────────────────

func (a *App) runGitLabDeviceFlow() {
	if gitlabClientID == "" {
		wailsruntime.EventsEmit(a.ctx, oauthEventError,
			"GitLab OAuth is not configured in this build. Contact the Minions team.")
		return
	}

	req, _ := http.NewRequest("POST", "https://gitlab.com/oauth/authorize_device",
		strings.NewReader(url.Values{
			"client_id": {gitlabClientID},
			"scope":     {"read_repository write_repository"},
		}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		wailsruntime.EventsEmit(a.ctx, oauthEventError, "GitLab request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	var dc struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
		Error           string `json:"error"`
		ErrorDesc       string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dc); err != nil || dc.DeviceCode == "" {
		wailsruntime.EventsEmit(a.ctx, oauthEventError, "Unexpected response from GitLab.")
		return
	}
	if dc.Error != "" {
		wailsruntime.EventsEmit(a.ctx, oauthEventError, "GitLab: "+dc.ErrorDesc)
		return
	}

	interval := dc.Interval
	if interval == 0 {
		interval = 5
	}

	wailsruntime.EventsEmit(a.ctx, oauthEventDeviceCode, DeviceCodePayload{
		UserCode:        dc.UserCode,
		VerificationURL: dc.VerificationURI,
	})
	_ = openBrowser(dc.VerificationURI)

	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(time.Duration(interval) * time.Second)

		token, slowDown, errMsg := pollGitLabToken(gitlabClientID, dc.DeviceCode)
		if slowDown {
			interval += 5
			continue
		}
		if errMsg != "" {
			wailsruntime.EventsEmit(a.ctx, oauthEventError, errMsg)
			return
		}
		if token != "" {
			a.gitHost = "gitlab.com"
			a.gitUsername = "oauth2"
			a.gitToken = token
			wailsruntime.EventsEmit(a.ctx, oauthEventComplete, "")
			return
		}
	}

	wailsruntime.EventsEmit(a.ctx, oauthEventError, "Code expired. Please try again.")
}

func pollGitLabToken(clientID, deviceCode string) (token string, slowDown bool, errMsg string) {
	resp, err := http.PostForm("https://gitlab.com/oauth/token", url.Values{
		"client_id":   {clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	})
	if err != nil {
		return "", false, ""
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", false, ""
	}

	if result.AccessToken != "" {
		return result.AccessToken, false, ""
	}

	switch result.Error {
	case "", "authorization_pending":
		return "", false, ""
	case "slow_down":
		return "", true, ""
	case "expired_token":
		return "", false, "Code expired. Please try again."
	case "access_denied":
		return "", false, "Authorization was denied."
	default:
		return "", false, "GitLab: " + result.ErrorDesc
	}
}

// ── PKCE helpers ──────────────────────────────────────────────────────────────

// newPKCEPair generates a random code_verifier and its S256 code_challenge.
func newPKCEPair() (verifier, challenge string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return
}
