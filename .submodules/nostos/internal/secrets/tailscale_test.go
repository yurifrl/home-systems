package secrets

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// scriptedResolver returns canned values for known refs. Any unknown ref
// returns an error that the caller can detect.
type scriptedResolver struct {
	values map[string]string
	calls  []string
}

func (s *scriptedResolver) ResolveRef(ref string) (string, error) {
	s.calls = append(s.calls, ref)
	if v, ok := s.values[ref]; ok {
		return v, nil
	}
	return "", errors.New("unknown ref: " + ref)
}

func newTestServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func newBackend(srv *httptest.Server, r RefResolver) *TailscaleBackend {
	url := ""
	if srv != nil {
		url = srv.URL
	}
	return NewTailscale(TailscaleConfig{
		OAuthClientIDRef:     "op://vault/item/client_id",
		OAuthClientSecretRef: "op://vault/item/client_secret",
		Tags:                 []string{"tag:home-systems"},
		ExpirySeconds:        7776000,
		Reusable:             false,
		Ephemeral:            false,
		Preauthorized:        true,
		Description:          "nostos",
	}, r, url)
}

func goodResolver() *scriptedResolver {
	return &scriptedResolver{values: map[string]string{
		"op://vault/item/client_id":     "client-abc",
		"op://vault/item/client_secret": "super-secret-value",
	}}
}

func mintedKey(t *testing.T, w http.ResponseWriter, r *http.Request, body map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatal(err)
	}
}

func TestTailscaleResolveSuccess(t *testing.T) {
	gotMintBody := map[string]any{}
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/oauth/token":
			if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
				t.Errorf("oauth content-type = %q", ct)
			}
			mintedKey(t, w, r, map[string]any{
				"access_token": "tok-1",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/api/v2/tailnet/-/keys":
			if got := r.Header.Get("Authorization"); got != "Bearer tok-1" {
				t.Errorf("auth header = %q", got)
			}
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &gotMintBody); err != nil {
				t.Fatal(err)
			}
			mintedKey(t, w, r, map[string]any{
				"id":  "k123",
				"key": "tskey-auth-fake-XYZ",
			})
		default:
			http.NotFound(w, r)
		}
	})

	r := goodResolver()
	b := newBackend(srv, r)
	val, err := b.Resolve("tailscale://authkey")
	if err != nil {
		t.Fatal(err)
	}
	if val != "tskey-auth-fake-XYZ" {
		t.Errorf("got key %q", val)
	}
	caps, _ := gotMintBody["capabilities"].(map[string]any)
	devices, _ := caps["devices"].(map[string]any)
	create, _ := devices["create"].(map[string]any)
	if v, _ := create["preauthorized"].(bool); !v {
		t.Errorf("preauthorized default not honored: %v", create)
	}
	if v, _ := gotMintBody["expirySeconds"].(float64); int(v) != 7776000 {
		t.Errorf("expiry default not honored: %v", v)
	}
	if v, _ := gotMintBody["description"].(string); v != "nostos" {
		t.Errorf("description default not honored: %v", v)
	}
}

func TestTailscaleResolveQueryOverrides(t *testing.T) {
	var gotBody map[string]any
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/oauth/token":
			mintedKey(t, w, r, map[string]any{"access_token": "tok"})
		case "/api/v2/tailnet/-/keys":
			b, _ := io.ReadAll(r.Body)
			gotBody = map[string]any{}
			_ = json.Unmarshal(b, &gotBody)
			mintedKey(t, w, r, map[string]any{"id": "k1", "key": "tskey-auth-Q"})
		default:
			http.NotFound(w, r)
		}
	})
	b := newBackend(srv, goodResolver())
	_, err := b.Resolve("tailscale://authkey?tags=tag:home-systems,tag:worker&expiry=86400&reusable=true&description=tp1-x")
	if err != nil {
		t.Fatal(err)
	}
	caps := gotBody["capabilities"].(map[string]any)
	create := caps["devices"].(map[string]any)["create"].(map[string]any)
	if v := create["reusable"].(bool); !v {
		t.Errorf("reusable override lost: %v", create)
	}
	tags := create["tags"].([]any)
	if len(tags) != 2 || tags[0].(string) != "tag:home-systems" || tags[1].(string) != "tag:worker" {
		t.Errorf("tags override lost: %v", tags)
	}
	if int(gotBody["expirySeconds"].(float64)) != 86400 {
		t.Errorf("expiry override lost: %v", gotBody["expirySeconds"])
	}
	if gotBody["description"].(string) != "tp1-x" {
		t.Errorf("description override lost: %v", gotBody["description"])
	}
}

func TestTailscaleOAuthFailure(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"bad creds"}`))
	})
	b := newBackend(srv, goodResolver())
	_, err := b.Resolve("tailscale://authkey")
	if err == nil {
		t.Fatal("expected oauth failure")
	}
	if !strings.Contains(err.Error(), "tailscale://authkey") {
		t.Errorf("URI missing from err: %v", err)
	}
	if strings.Contains(err.Error(), "super-secret-value") {
		t.Errorf("client secret leaked: %v", err)
	}
}

func TestTailscaleMintFailureRedactsResponseSecrets(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/oauth/token":
			mintedKey(t, w, r, map[string]any{"access_token": "tok"})
		default:
			// Some imaginary error response that echoes a partial key
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"would have minted tskey-auth-PARTIAL but ACL denied"}`))
		}
	})
	b := newBackend(srv, goodResolver())
	_, err := b.Resolve("tailscale://authkey")
	if err == nil {
		t.Fatal("expected mint failure")
	}
	if strings.Contains(err.Error(), "tskey-auth-PARTIAL") {
		t.Errorf("token leaked through error: %v", err)
	}
	if !strings.Contains(err.Error(), "<REDACTED>") {
		t.Errorf("expected redaction marker in: %v", err)
	}
	if !strings.Contains(err.Error(), "tailscale://authkey") {
		t.Errorf("URI missing from err: %v", err)
	}
}

func TestTailscaleRejectsUnknownPath(t *testing.T) {
	// no test server should be hit
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be called, got %s", r.URL.Path)
	})
	b := newBackend(srv, goodResolver())
	_, err := b.Resolve("tailscale://devices")
	if err == nil {
		t.Fatal("expected rejection for unknown path")
	}
	if !strings.Contains(err.Error(), "tailscale://devices") {
		t.Errorf("URI missing from err: %v", err)
	}
	if !strings.Contains(err.Error(), "authkey") {
		t.Errorf("err should mention only-supported resource: %v", err)
	}
}

func TestTailscaleResolverFailureNoNetwork(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("network must not be hit when ref resolution fails; got %s", r.URL.Path)
	})
	r := &scriptedResolver{values: map[string]string{}} // no creds
	b := newBackend(srv, r)
	_, err := b.Resolve("tailscale://authkey")
	if err == nil {
		t.Fatal("expected resolver failure")
	}
	if !strings.Contains(err.Error(), "tailscale://authkey") {
		t.Errorf("URI missing from err: %v", err)
	}
	// error must reference the ref so the operator can fix config
	if !strings.Contains(err.Error(), "op://vault/item/client_id") {
		t.Errorf("ref missing from err: %v", err)
	}
}

func TestTailscaleValidateSuccess(t *testing.T) {
	hits := 0
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/api/v2/oauth/token" {
			t.Errorf("Validate must only hit oauth, got %s", r.URL.Path)
		}
		mintedKey(t, w, r, map[string]any{"access_token": "tok"})
	})
	b := newBackend(srv, goodResolver())
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Errorf("expected 1 oauth call, got %d", hits)
	}
}

func TestTailscaleValidateMissingResolver(t *testing.T) {
	b := NewTailscale(TailscaleConfig{
		OAuthClientIDRef:     "op://a/b/c",
		OAuthClientSecretRef: "op://a/b/d",
	}, nil, "http://127.0.0.1:1")
	err := b.Validate()
	if err == nil {
		t.Fatal("expected error for missing resolver")
	}
}

func TestRedactToken(t *testing.T) {
	in := `error: minted tskey-auth-ABCDEF12345 then failed; also tskey-api-XYZ987,end`
	out := redactToken(in)
	if strings.Contains(out, "tskey-auth-ABCDEF12345") {
		t.Errorf("not redacted: %q", out)
	}
	if strings.Contains(out, "tskey-api-XYZ987") {
		t.Errorf("not redacted: %q", out)
	}
}
