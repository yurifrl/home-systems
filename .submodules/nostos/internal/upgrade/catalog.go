package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Doer is the minimal HTTP client surface FetchCatalog needs. *http.Client
// satisfies it; tests inject a stub.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

const talosReleasesURL = "https://api.github.com/repos/siderolabs/talos/releases?per_page=100"

// ghRelease is the subset of the GitHub releases payload we consume.
type ghRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
}

// isPrerelease reports whether a tag is an alpha/beta/rc pre-release.
func isPrerelease(tag string) bool {
	t := strings.ToLower(tag)
	return strings.Contains(t, "-alpha") ||
		strings.Contains(t, "-beta") ||
		strings.Contains(t, "-rc") ||
		strings.Contains(t, "alpha") ||
		strings.Contains(t, "beta") ||
		strings.Contains(t, "rc")
}

// parseCatalog decodes the GitHub releases JSON, dropping pre-releases and
// unparseable tags. Pure helper so it can be unit-tested with a fixture.
func parseCatalog(body []byte) ([]Version, error) {
	var releases []ghRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("parse releases JSON: %w", err)
	}
	var out []Version
	for _, r := range releases {
		if r.Prerelease || isPrerelease(r.TagName) {
			continue
		}
		v, err := ParseVersion(r.TagName)
		if err != nil {
			continue // skip tags that aren't simple semver
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Less(out[j]) })
	return out, nil
}

// FetchCatalog queries the siderolabs/talos GitHub releases and returns the
// stable Versions (pre-releases excluded), sorted ascending. The HTTP client
// is injectable; pass nil to use a default client with a sane timeout.
func FetchCatalog(ctx context.Context, client Doer) ([]Version, error) {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, talosReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch talos releases: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read talos releases: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("talos releases API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return parseCatalog(body)
}
