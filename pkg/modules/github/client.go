// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package github

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"strconv"
	"sync"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/models"

	"github.com/golang-jwt/jwt/v5"
)

// defaultBaseURL is a var so tests can point the client construction at a
// stub server; production code always uses github.com.
var defaultBaseURL = "https://api.github.com"

const (
	perPage = 100
	// GitHub App installation tokens are valid for one hour; mint a new one
	// shortly before expiry so a sync never runs on a token that dies mid-run.
	tokenRefreshLead = 5 * time.Minute
	appJWTLifetime   = 10 * time.Minute
)

// errNotFound and errUnauthorized let the sync engine distinguish
// "repo is gone / token revoked" from transient failures without string
// matching on GitHub's response bodies.
var (
	errNotFound     = errors.New("github: not found")
	errUnauthorized = errors.New("github: unauthorized")
)

// credential produces the Authorization header value for API requests.
type credential interface {
	authorization(ctx context.Context) (string, error)
}

// patCredential is a static personal access token.
type patCredential string

func (p patCredential) authorization(_ context.Context) (string, error) {
	return "Bearer " + string(p), nil
}

// installationCredential mints and caches installation access tokens for a
// GitHub App installation, signing app JWTs with the instance's private key.
type installationCredential struct {
	installationID int64

	mu         sync.Mutex
	token      string
	expiresAt  time.Time
	appID      string
	privateKey *rsa.PrivateKey
	httpClient *http.Client
	baseURL    string
}

func (c *installationCredential) authorization(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.expiresAt.Add(-tokenRefreshLead)) {
		return "Bearer " + c.token, nil
	}

	if err := c.loadAppCredentials(); err != nil {
		return "", err
	}

	appJWT, err := c.signAppJWT()
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/app/installations/"+strconv.FormatInt(c.installationID, 10)+"/access_tokens", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("github: minting installation token: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("github: minting installation token: %s (%d)", string(body), resp.StatusCode)
	}

	var tokenResp accessTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("github: decoding installation token: %w", err)
	}
	c.token = tokenResp.Token
	c.expiresAt = tokenResp.ExpiresAt
	return "Bearer " + c.token, nil
}

// loadAppCredentials reads the app id and private key lazily (config may not
// be loaded when a credential struct is built) and caches them.
func (c *installationCredential) loadAppCredentials() error {
	if c.privateKey != nil && c.appID != "" {
		return nil
	}

	appID := config.GithubAppID.GetString()
	if appID == "" {
		return errors.New("github: github.appid must be set for GitHub App connections")
	}
	pemData := config.GithubAppPrivateKey.GetString()
	// A github.appprivatekey.file path takes precedence over the inline key.
	if fromFile := config.GetConfigValueFromFile(string(config.GithubAppPrivateKey)); fromFile != "" {
		pemData = fromFile
	}
	if pemData == "" {
		return errors.New("github: github.appprivatekey must be set for GitHub App connections")
	}
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return errors.New("github: github.appprivatekey is not a valid PEM key")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		var parsed any
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("github: parsing app private key: %w", err)
		}
		rsaKey, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return errors.New("github: app private key is not an RSA key")
		}
		key = rsaKey
	}
	c.appID = appID
	c.privateKey = key
	return nil
}

func (c *installationCredential) signAppJWT() (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": c.appID,
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(appJWTLifetime).Unix(),
	})
	return token.SignedString(c.privateKey)
}

// Client talks to the GitHub REST API with a PAT or installation credential.
type Client struct {
	baseURL    string
	httpClient *http.Client
	cred       credential
}

// NewClient returns a client for the given credential.
func NewClient(baseURL string, cred credential, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{baseURL: baseURL, httpClient: httpClient, cred: cred}
}

// NewClientForConnection builds a client authenticating as the connection
// requires: installation token for App connections, the stored PAT otherwise.
func NewClientForConnection(conn *models.GitHubConnection) *Client {
	var cred credential
	if conn.InstallationID > 0 {
		cred = &installationCredential{installationID: conn.InstallationID}
	} else {
		cred = patCredential(conn.Token)
	}
	return NewClient(defaultBaseURL, cred, nil)
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	if len(query) > 0 {
		req.URL.RawQuery = query.Encode()
	}
	auth, err := c.cred.authorization(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: %s %s: %w", method, path, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("github: reading response of %s %s: %w", method, path, err)
	}

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%w: %s %s", errNotFound, method, path)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%w: %s %s (%d): %s", errUnauthorized, method, path, resp.StatusCode, snippet(raw))
	case resp.StatusCode >= 400:
		return nil, fmt.Errorf("github: %s %s: %d: %s", method, path, resp.StatusCode, snippet(raw))
	}
	return raw, nil
}

func snippet(b []byte) string {
	if len(b) > 200 {
		b = b[:200]
	}
	return string(b)
}

// GetRepo fetches a repository, validating that the credential can see it.
func (c *Client) GetRepo(ctx context.Context, owner, name string) (*Repository, error) {
	raw, err := c.do(ctx, http.MethodGet, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(name), nil, nil)
	if err != nil {
		return nil, err
	}
	repo := &Repository{}
	if err := json.Unmarshal(raw, repo); err != nil {
		return nil, fmt.Errorf("github: decoding repository: %w", err)
	}
	return repo, nil
}

// ListIssues returns all issues (including PRs — that's how the issues API
// reports them) of a repo, updated after since (zero = everything), oldest
// first so backfills create parents before relations reference them.
func (c *Client) ListIssues(ctx context.Context, owner, name string, since time.Time) ([]*Issue, error) {
	query := url.Values{}
	query.Set("state", "all")
	query.Set("sort", "created")
	query.Set("direction", "asc")
	query.Set("per_page", strconv.Itoa(perPage))
	if !since.IsZero() {
		query.Set("since", since.UTC().Format(time.RFC3339))
	}

	var all []*Issue
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name) + "/issues"
	for page := 1; ; page++ {
		query.Set("page", strconv.Itoa(page))
		raw, err := c.do(ctx, http.MethodGet, path, query, nil)
		if err != nil {
			return nil, err
		}
		batch := []*Issue{}
		if err := json.Unmarshal(raw, &batch); err != nil {
			return nil, fmt.Errorf("github: decoding issues: %w", err)
		}
		all = append(all, batch...)
		if len(batch) < perPage {
			return all, nil
		}
	}
}

// GetIssue fetches one issue or PR by number.
func (c *Client) GetIssue(ctx context.Context, owner, name string, number int64) (*Issue, error) {
	raw, err := c.do(ctx, http.MethodGet,
		"/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(name)+"/issues/"+strconv.FormatInt(number, 10), nil, nil)
	if err != nil {
		return nil, err
	}
	issue := &Issue{}
	if err := json.Unmarshal(raw, issue); err != nil {
		return nil, fmt.Errorf("github: decoding issue: %w", err)
	}
	return issue, nil
}

// CreateRepoWebhook registers a repository webhook for PAT connections so
// GitHub pushes updates instead of waiting for the next reconciliation.
func (c *Client) CreateRepoWebhook(ctx context.Context, owner, name, targetURL, secret string) (int64, error) {
	body := map[string]any{
		"config": map[string]any{
			"url":          targetURL,
			"content_type": "json",
			"secret":       secret,
			"insecure_ssl": "0",
		},
		"events": []string{"issues", "pull_request"},
	}
	raw, err := c.do(ctx, http.MethodPost, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(name)+"/hooks", nil, body)
	if err != nil {
		return 0, err
	}
	hook := &repoHook{}
	if err := json.Unmarshal(raw, hook); err != nil {
		return 0, fmt.Errorf("github: decoding webhook response: %w", err)
	}
	return hook.ID, nil
}

// DeleteRepoWebhook removes a previously created repository webhook.
func (c *Client) DeleteRepoWebhook(ctx context.Context, owner, name string, id int64) error {
	_, err := c.do(ctx, http.MethodDelete,
		"/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(name)+"/hooks/"+strconv.FormatInt(id, 10), nil, nil)
	return err
}

// SetBaseURLForTesting redirects all API calls to a stub server and returns a
// restore function. Production code never calls this; it exists so test
// packages outside this one (webtests) can exercise the routes end-to-end
// against a fake GitHub.
func SetBaseURLForTesting(u string) (restore func()) {
	old := defaultBaseURL
	defaultBaseURL = u
	return func() {
		defaultBaseURL = old
	}
}
