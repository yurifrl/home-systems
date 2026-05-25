package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultTailscaleBaseURL is the canonical Tailscale API endpoint.
const DefaultTailscaleBaseURL = "https://api.tailscale.com"

// RefResolver resolves an arbitrary credential ref (e.g. op://...) to its
// plain value via the registered backends. The Tailscale backend uses this
// indirection so it never has to know about op:// or any other scheme.
type RefResolver interface {
	ResolveRef(ref string) (string, error)
}

// RefResolverFunc adapts a plain function to RefResolver.
type RefResolverFunc func(ref string) (string, error)

// ResolveRef implements RefResolver.
func (f RefResolverFunc) ResolveRef(ref string) (string, error) { return f(ref) }

// TailscaleBackend mints ephemeral / preauthorized auth keys against the
// Tailscale API using an OAuth client (client_credentials grant, scope=auth_keys).
//
// It implements Backend.Resolve only for the URI shape `tailscale://authkey`
// (with optional query overrides). All other paths are rejected with a clear
// error so typos surface loudly during template render.
//
// The backend never caches the OAuth access token across Resolve calls — each
// call mints a fresh token, uses it once, and discards it. This keeps the
// blast-radius of any leaked token to a single render.
type TailscaleBackend struct {
	BaseURL string
	Tailnet string // "-" means "the default tailnet for the OAuth client"

	OAuthClientIDRef     string
	OAuthClientSecretRef string

	DefaultTags          []string
	DefaultExpirySeconds int
	DefaultReusable      bool
	DefaultEphemeral     bool
	DefaultPreauthorized bool
	DefaultDescription   string

	Resolver   RefResolver
	HTTPClient *http.Client
}

// TailscaleConfig captures the values needed to construct a TailscaleBackend.
// Mirrors the YAML schema under secrets.tailscale.
type TailscaleConfig struct {
	OAuthClientIDRef     string
	OAuthClientSecretRef string
	Tags                 []string
	ExpirySeconds        int
	Reusable             bool
	Ephemeral            bool
	Preauthorized        bool
	Description          string
	Tailnet              string
}

// NewTailscale constructs a backend. baseURL may be "" to use the public API.
func NewTailscale(cfg TailscaleConfig, resolver RefResolver, baseURL string) *TailscaleBackend {
	if baseURL == "" {
		baseURL = DefaultTailscaleBaseURL
	}
	tailnet := cfg.Tailnet
	if tailnet == "" {
		tailnet = "-"
	}
	expiry := cfg.ExpirySeconds
	if expiry == 0 {
		expiry = 7776000 // 90 days
	}
	desc := cfg.Description
	if desc == "" {
		desc = "nostos"
	}
	return &TailscaleBackend{
		BaseURL:              strings.TrimRight(baseURL, "/"),
		Tailnet:              tailnet,
		OAuthClientIDRef:     cfg.OAuthClientIDRef,
		OAuthClientSecretRef: cfg.OAuthClientSecretRef,
		DefaultTags:          cfg.Tags,
		DefaultExpirySeconds: expiry,
		DefaultReusable:      cfg.Reusable,
		DefaultEphemeral:     cfg.Ephemeral,
		DefaultPreauthorized: cfg.Preauthorized,
		DefaultDescription:   desc,
		Resolver:             resolver,
		HTTPClient:           &http.Client{Timeout: 30 * time.Second},
	}
}

// Scheme implements Backend.
func (b *TailscaleBackend) Scheme() string { return "tailscale" }

// Validate hits the OAuth token endpoint to confirm creds + scope without
// minting (and thus without leaving an audit trail of unused keys).
func (b *TailscaleBackend) Validate() error {
	const uri = "tailscale://authkey"
	if b.OAuthClientIDRef == "" || b.OAuthClientSecretRef == "" {
		return &ResolveError{URI: uri, Reason: "tailscale: oauth_client_id_ref and oauth_client_secret_ref must be set"}
	}
	if b.Resolver == nil {
		return &ResolveError{URI: uri, Reason: "tailscale: no ref resolver configured"}
	}
	id, err := b.Resolver.ResolveRef(b.OAuthClientIDRef)
	if err != nil {
		return &ResolveError{URI: uri, Reason: fmt.Sprintf("resolving oauth_client_id_ref %s: %v", b.OAuthClientIDRef, err)}
	}
	secret, err := b.Resolver.ResolveRef(b.OAuthClientSecretRef)
	if err != nil {
		return &ResolveError{URI: uri, Reason: fmt.Sprintf("resolving oauth_client_secret_ref %s: %v", b.OAuthClientSecretRef, err)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := b.fetchAccessToken(ctx, id, secret); err != nil {
		return &ResolveError{URI: uri, Reason: err.Error()}
	}
	return nil
}

// Resolve mints a new auth key. The URI must be tailscale://authkey with
// optional overrides via query string.
func (b *TailscaleBackend) Resolve(uri string) (string, error) {
	opts, err := parseTailscaleURI(uri, b)
	if err != nil {
		return "", err
	}
	if b.Resolver == nil {
		return "", &ResolveError{URI: uri, Reason: "tailscale: no ref resolver configured"}
	}
	id, err := b.Resolver.ResolveRef(b.OAuthClientIDRef)
	if err != nil {
		return "", &ResolveError{URI: uri, Reason: fmt.Sprintf("resolving oauth_client_id_ref %s: %v", b.OAuthClientIDRef, err)}
	}
	secret, err := b.Resolver.ResolveRef(b.OAuthClientSecretRef)
	if err != nil {
		return "", &ResolveError{URI: uri, Reason: fmt.Sprintf("resolving oauth_client_secret_ref %s: %v", b.OAuthClientSecretRef, err)}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	token, err := b.fetchAccessToken(ctx, id, secret)
	if err != nil {
		return "", &ResolveError{URI: uri, Reason: err.Error()}
	}
	key, _, err := b.mintKey(ctx, token, opts)
	if err != nil {
		return "", &ResolveError{URI: uri, Reason: err.Error()}
	}
	return key, nil
}

// MintKeyOpts are the per-request key-creation knobs.
type MintKeyOpts struct {
	Reusable      bool
	Ephemeral     bool
	Preauthorized bool
	Tags          []string
	ExpirySeconds int
	Description   string
}

func parseTailscaleURI(uri string, b *TailscaleBackend) (MintKeyOpts, error) {
	if !strings.HasPrefix(uri, "tailscale://") {
		return MintKeyOpts{}, &ResolveError{URI: uri, Reason: "not a tailscale:// URI"}
	}
	u, err := url.Parse(uri)
	if err != nil {
		return MintKeyOpts{}, &ResolveError{URI: uri, Reason: fmt.Sprintf("invalid URI: %v", err)}
	}
	// url.Parse treats "tailscale://authkey" as Host=authkey, Path="".
	resource := u.Host
	if resource == "" {
		resource = strings.TrimPrefix(u.Path, "/")
	}
	if resource != "authkey" {
		return MintKeyOpts{}, &ResolveError{URI: uri, Reason: fmt.Sprintf("unsupported tailscale resource %q (only \"authkey\" is supported)", resource)}
	}

	opts := MintKeyOpts{
		Reusable:      b.DefaultReusable,
		Ephemeral:     b.DefaultEphemeral,
		Preauthorized: b.DefaultPreauthorized,
		Tags:          append([]string{}, b.DefaultTags...),
		ExpirySeconds: b.DefaultExpirySeconds,
		Description:   b.DefaultDescription,
	}
	q := u.Query()
	if v := q.Get("tags"); v != "" {
		parts := strings.Split(v, ",")
		opts.Tags = opts.Tags[:0]
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				opts.Tags = append(opts.Tags, p)
			}
		}
	}
	if v := q.Get("expiry"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return MintKeyOpts{}, &ResolveError{URI: uri, Reason: fmt.Sprintf("invalid expiry %q (want positive seconds)", v)}
		}
		opts.ExpirySeconds = n
	}
	if v := q.Get("description"); v != "" {
		opts.Description = v
	}
	if v := q.Get("reusable"); v != "" {
		bv, err := strconv.ParseBool(v)
		if err != nil {
			return MintKeyOpts{}, &ResolveError{URI: uri, Reason: fmt.Sprintf("invalid reusable %q", v)}
		}
		opts.Reusable = bv
	}
	if v := q.Get("ephemeral"); v != "" {
		bv, err := strconv.ParseBool(v)
		if err != nil {
			return MintKeyOpts{}, &ResolveError{URI: uri, Reason: fmt.Sprintf("invalid ephemeral %q", v)}
		}
		opts.Ephemeral = bv
	}
	if v := q.Get("preauthorized"); v != "" {
		bv, err := strconv.ParseBool(v)
		if err != nil {
			return MintKeyOpts{}, &ResolveError{URI: uri, Reason: fmt.Sprintf("invalid preauthorized %q", v)}
		}
		opts.Preauthorized = bv
	}
	return opts, nil
}

// fetchAccessToken performs the client_credentials grant.
func (b *TailscaleBackend) fetchAccessToken(ctx context.Context, clientID, clientSecret string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("scope", "auth_keys")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)

	endpoint := b.BaseURL + "/api/v2/oauth/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build oauth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := b.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("oauth token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oauth token endpoint returned %d: %s", resp.StatusCode, redactToken(strings.TrimSpace(string(body))))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("parse oauth token response: %w", err)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("oauth token response missing access_token")
	}
	return out.AccessToken, nil
}

// MintedKey is the (subset of the) Tailscale API response for key creation.
type MintedKey struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Created     time.Time `json:"created,omitempty"`
	Expires     time.Time `json:"expires,omitempty"`
	Description string    `json:"description,omitempty"`
}

func (b *TailscaleBackend) mintKey(ctx context.Context, token string, opts MintKeyOpts) (string, string, error) {
	body := map[string]any{
		"capabilities": map[string]any{
			"devices": map[string]any{
				"create": map[string]any{
					"reusable":      opts.Reusable,
					"ephemeral":     opts.Ephemeral,
					"preauthorized": opts.Preauthorized,
					"tags":          opts.Tags,
				},
			},
		},
		"expirySeconds": opts.ExpirySeconds,
		"description":   opts.Description,
	}
	enc, err := json.Marshal(body)
	if err != nil {
		return "", "", fmt.Errorf("marshal key request: %w", err)
	}
	endpoint := fmt.Sprintf("%s/api/v2/tailnet/%s/keys", b.BaseURL, b.Tailnet)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(enc))
	if err != nil {
		return "", "", fmt.Errorf("build mint key request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := b.httpClient().Do(req)
	if err != nil {
		return "", "", fmt.Errorf("mint key request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", "", fmt.Errorf("tailscale create-key returned %d: %s", resp.StatusCode, redactToken(strings.TrimSpace(string(respBody))))
	}
	var out MintedKey
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", "", fmt.Errorf("parse create-key response: %w", err)
	}
	if out.Key == "" || out.ID == "" {
		return "", "", fmt.Errorf("create-key response missing key/id")
	}
	return out.Key, out.ID, nil
}

// MintKey is exposed for the `secrets test` smoke flow. It returns the minted
// key value (which the caller is responsible for revoking).
func (b *TailscaleBackend) MintKey(ctx context.Context, opts MintKeyOpts) (MintedKey, error) {
	if b.Resolver == nil {
		return MintedKey{}, fmt.Errorf("tailscale: no ref resolver configured")
	}
	id, err := b.Resolver.ResolveRef(b.OAuthClientIDRef)
	if err != nil {
		return MintedKey{}, fmt.Errorf("resolving oauth_client_id_ref %s: %w", b.OAuthClientIDRef, err)
	}
	secret, err := b.Resolver.ResolveRef(b.OAuthClientSecretRef)
	if err != nil {
		return MintedKey{}, fmt.Errorf("resolving oauth_client_secret_ref %s: %w", b.OAuthClientSecretRef, err)
	}
	token, err := b.fetchAccessToken(ctx, id, secret)
	if err != nil {
		return MintedKey{}, err
	}
	if opts.ExpirySeconds == 0 {
		opts.ExpirySeconds = b.DefaultExpirySeconds
	}
	if len(opts.Tags) == 0 {
		opts.Tags = append([]string{}, b.DefaultTags...)
	}
	if opts.Description == "" {
		opts.Description = b.DefaultDescription
	}
	body := map[string]any{
		"capabilities": map[string]any{
			"devices": map[string]any{
				"create": map[string]any{
					"reusable":      opts.Reusable,
					"ephemeral":     opts.Ephemeral,
					"preauthorized": opts.Preauthorized,
					"tags":          opts.Tags,
				},
			},
		},
		"expirySeconds": opts.ExpirySeconds,
		"description":   opts.Description,
	}
	enc, err := json.Marshal(body)
	if err != nil {
		return MintedKey{}, fmt.Errorf("marshal key request: %w", err)
	}
	endpoint := fmt.Sprintf("%s/api/v2/tailnet/%s/keys", b.BaseURL, b.Tailnet)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(enc))
	if err != nil {
		return MintedKey{}, fmt.Errorf("build mint key request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := b.httpClient().Do(req)
	if err != nil {
		return MintedKey{}, fmt.Errorf("mint key request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return MintedKey{}, fmt.Errorf("tailscale create-key returned %d: %s", resp.StatusCode, redactToken(strings.TrimSpace(string(respBody))))
	}
	var out MintedKey
	if err := json.Unmarshal(respBody, &out); err != nil {
		return MintedKey{}, fmt.Errorf("parse create-key response: %w", err)
	}
	if out.Key == "" || out.ID == "" {
		return MintedKey{}, fmt.Errorf("create-key response missing key/id")
	}
	return out, nil
}

// KeyInfo is one entry in /tailnet/-/keys list responses.
type KeyInfo struct {
	ID          string    `json:"id"`
	Description string    `json:"description,omitempty"`
	Created     time.Time `json:"created,omitempty"`
	Expires     time.Time `json:"expires,omitempty"`
	Revoked     time.Time `json:"revoked,omitempty"`
	Used        bool      `json:"used,omitempty"`
	Tags        []string  `json:"tags,omitempty"` // some tailscale responses inline tags
	Capabilities struct {
		Devices struct {
			Create struct {
				Tags []string `json:"tags,omitempty"`
			} `json:"create"`
		} `json:"devices"`
	} `json:"capabilities,omitempty"`
}

// EffectiveTags returns capabilities.devices.create.tags, falling back to top-level Tags.
func (k KeyInfo) EffectiveTags() []string {
	if len(k.Capabilities.Devices.Create.Tags) > 0 {
		return k.Capabilities.Devices.Create.Tags
	}
	return k.Tags
}

// ListKeys hits GET /tailnet/-/keys.
func (b *TailscaleBackend) ListKeys(ctx context.Context) ([]KeyInfo, error) {
	if b.Resolver == nil {
		return nil, fmt.Errorf("tailscale: no ref resolver configured")
	}
	id, err := b.Resolver.ResolveRef(b.OAuthClientIDRef)
	if err != nil {
		return nil, fmt.Errorf("resolving oauth_client_id_ref %s: %w", b.OAuthClientIDRef, err)
	}
	secret, err := b.Resolver.ResolveRef(b.OAuthClientSecretRef)
	if err != nil {
		return nil, fmt.Errorf("resolving oauth_client_secret_ref %s: %w", b.OAuthClientSecretRef, err)
	}
	token, err := b.fetchAccessToken(ctx, id, secret)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/api/v2/tailnet/%s/keys", b.BaseURL, b.Tailnet)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := b.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("list keys request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tailscale list-keys returned %d: %s", resp.StatusCode, redactToken(strings.TrimSpace(string(body))))
	}
	// API returns either {"keys":[{"id":...}]} (list endpoint) — possibly with
	// only id+description. We then GET each key for full info.
	var listResp struct {
		Keys []KeyInfo `json:"keys"`
	}
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("parse list-keys response: %w", err)
	}
	// Best-effort enrich: GET each key for capabilities/expiry.
	out := make([]KeyInfo, 0, len(listResp.Keys))
	for _, k := range listResp.Keys {
		full, err := b.getKey(ctx, token, k.ID)
		if err != nil {
			// fall back to the partial entry rather than failing the whole list
			out = append(out, k)
			continue
		}
		out = append(out, full)
	}
	return out, nil
}

func (b *TailscaleBackend) getKey(ctx context.Context, token, keyID string) (KeyInfo, error) {
	endpoint := fmt.Sprintf("%s/api/v2/tailnet/%s/keys/%s", b.BaseURL, b.Tailnet, url.PathEscape(keyID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return KeyInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := b.httpClient().Do(req)
	if err != nil {
		return KeyInfo{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return KeyInfo{}, fmt.Errorf("get key %s returned %d", keyID, resp.StatusCode)
	}
	var k KeyInfo
	if err := json.Unmarshal(body, &k); err != nil {
		return KeyInfo{}, err
	}
	if k.ID == "" {
		k.ID = keyID
	}
	return k, nil
}

// RevokeKey deletes a key by id via DELETE /tailnet/-/keys/<id>.
func (b *TailscaleBackend) RevokeKey(ctx context.Context, keyID string) error {
	if b.Resolver == nil {
		return fmt.Errorf("tailscale: no ref resolver configured")
	}
	id, err := b.Resolver.ResolveRef(b.OAuthClientIDRef)
	if err != nil {
		return fmt.Errorf("resolving oauth_client_id_ref %s: %w", b.OAuthClientIDRef, err)
	}
	secret, err := b.Resolver.ResolveRef(b.OAuthClientSecretRef)
	if err != nil {
		return fmt.Errorf("resolving oauth_client_secret_ref %s: %w", b.OAuthClientSecretRef, err)
	}
	token, err := b.fetchAccessToken(ctx, id, secret)
	if err != nil {
		return err
	}
	return b.revokeKeyWithToken(ctx, token, keyID)
}

func (b *TailscaleBackend) revokeKeyWithToken(ctx context.Context, token, keyID string) error {
	endpoint := fmt.Sprintf("%s/api/v2/tailnet/%s/keys/%s", b.BaseURL, b.Tailnet, url.PathEscape(keyID))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := b.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("revoke key request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("tailscale delete-key returned %d: %s", resp.StatusCode, redactToken(strings.TrimSpace(string(body))))
	}
	return nil
}

// Device is a subset of /tailnet/-/devices entries.
type Device struct {
	ID         string    `json:"id"`
	NodeID     string    `json:"nodeId,omitempty"`
	Name       string    `json:"name,omitempty"`
	Hostname   string    `json:"hostname,omitempty"`
	Addresses  []string  `json:"addresses,omitempty"`
	LastSeen   time.Time `json:"lastSeen,omitempty"`
	Created    time.Time `json:"created,omitempty"`
	OS         string    `json:"os,omitempty"`
	User       string    `json:"user,omitempty"`
	Authorized bool      `json:"authorized,omitempty"`
}

// ListDevices fetches GET /api/v2/tailnet/{tailnet}/devices.
func (b *TailscaleBackend) ListDevices(ctx context.Context) ([]Device, error) {
	if b.Resolver == nil {
		return nil, fmt.Errorf("tailscale: no ref resolver configured")
	}
	id, err := b.Resolver.ResolveRef(b.OAuthClientIDRef)
	if err != nil {
		return nil, fmt.Errorf("resolving oauth_client_id_ref %s: %w", b.OAuthClientIDRef, err)
	}
	secret, err := b.Resolver.ResolveRef(b.OAuthClientSecretRef)
	if err != nil {
		return nil, fmt.Errorf("resolving oauth_client_secret_ref %s: %w", b.OAuthClientSecretRef, err)
	}
	token, err := b.fetchAccessToken(ctx, id, secret)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/api/v2/tailnet/%s/devices", b.BaseURL, b.Tailnet)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := b.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("list devices request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tailscale list-devices returned %d: %s", resp.StatusCode, redactToken(strings.TrimSpace(string(body))))
	}
	var out struct {
		Devices []Device `json:"devices"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse list-devices response: %w", err)
	}
	return out.Devices, nil
}

// DeleteDevice issues DELETE /api/v2/device/{id}.
func (b *TailscaleBackend) DeleteDevice(ctx context.Context, deviceID string) error {
	if b.Resolver == nil {
		return fmt.Errorf("tailscale: no ref resolver configured")
	}
	id, err := b.Resolver.ResolveRef(b.OAuthClientIDRef)
	if err != nil {
		return fmt.Errorf("resolving oauth_client_id_ref %s: %w", b.OAuthClientIDRef, err)
	}
	secret, err := b.Resolver.ResolveRef(b.OAuthClientSecretRef)
	if err != nil {
		return fmt.Errorf("resolving oauth_client_secret_ref %s: %w", b.OAuthClientSecretRef, err)
	}
	token, err := b.fetchAccessToken(ctx, id, secret)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/api/v2/device/%s", b.BaseURL, url.PathEscape(deviceID))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := b.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("delete device request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("tailscale delete-device returned %d: %s", resp.StatusCode, redactToken(strings.TrimSpace(string(body))))
	}
	return nil
}

func (b *TailscaleBackend) httpClient() *http.Client {
	if b.HTTPClient != nil {
		return b.HTTPClient
	}
	return http.DefaultClient
}

// redactToken scrubs anything that looks like a tailscale auth key or oauth
// access token from error messages so we never leak secrets through logs.
func redactToken(s string) string {
	for _, prefix := range []string{"tskey-auth-", "tskey-api-", "tskey-client-", "tskey-"} {
		for {
			idx := strings.Index(s, prefix)
			if idx < 0 {
				break
			}
			// find end of token (whitespace, quote, end)
			end := idx + len(prefix)
			for end < len(s) {
				c := s[end]
				if c == ' ' || c == '\t' || c == '\n' || c == '"' || c == '\'' || c == ',' || c == '}' || c == ']' {
					break
				}
				end++
			}
			s = s[:idx] + "<REDACTED>" + s[end:]
		}
	}
	return s
}
