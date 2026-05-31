package upgrade

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// stubDoer returns a canned response, capturing the request for assertions.
type stubDoer struct {
	body   string
	status int
	gotURL string
}

func (s *stubDoer) Do(req *http.Request) (*http.Response, error) {
	s.gotURL = req.URL.String()
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Header:     make(http.Header),
	}, nil
}

const releasesFixture = `[
  {"tag_name": "v1.13.3", "prerelease": false},
  {"tag_name": "v1.13.0-alpha.1", "prerelease": true},
  {"tag_name": "v1.12.8", "prerelease": false},
  {"tag_name": "v1.12.0-beta.0", "prerelease": false},
  {"tag_name": "v1.11.6", "prerelease": false},
  {"tag_name": "v1.11.0-rc.2", "prerelease": false},
  {"tag_name": "not-a-version", "prerelease": false}
]`

func TestFetchCatalogFiltersPrereleases(t *testing.T) {
	stub := &stubDoer{body: releasesFixture}
	got, err := FetchCatalog(context.Background(), stub)
	if err != nil {
		t.Fatal(err)
	}
	want := []Version{{1, 11, 6}, {1, 12, 8}, {1, 13, 3}}
	if len(got) != len(want) {
		t.Fatalf("got %d versions %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("catalog[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if stub.gotURL != talosReleasesURL {
		t.Errorf("requested URL = %q, want %q", stub.gotURL, talosReleasesURL)
	}
}

func TestParseCatalogSorts(t *testing.T) {
	got, err := parseCatalog([]byte(`[
		{"tag_name":"v1.13.3"},
		{"tag_name":"v1.10.1"},
		{"tag_name":"v1.12.0"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	want := []Version{{1, 10, 1}, {1, 12, 0}, {1, 13, 3}}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sorted[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}
