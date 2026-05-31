package upgrade

import (
	"reflect"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in      string
		want    Version
		wantErr bool
	}{
		{"v1.10.3", Version{1, 10, 3}, false},
		{"1.10.3", Version{1, 10, 3}, false},
		{"  v1.13.0 ", Version{1, 13, 0}, false},
		{"", Version{}, true},
		{"v1.10", Version{}, true},
		{"1.10.3.4", Version{}, true},
		{"vx.y.z", Version{}, true},
		{"v1.ten.3", Version{}, true},
	}
	for _, c := range cases {
		got, err := ParseVersion(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseVersion(%q): expected error, got %v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseVersion(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseVersion(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func mustParse(t *testing.T, s string) Version {
	t.Helper()
	v, err := ParseVersion(s)
	if err != nil {
		t.Fatalf("mustParse(%q): %v", s, err)
	}
	return v
}

func TestComputePathMultiMinor(t *testing.T) {
	catalog := []Version{
		mustParse(t, "1.11.6"),
		mustParse(t, "1.11.2"), // lower patch; should not be picked
		mustParse(t, "1.12.8"),
		mustParse(t, "1.13.3"),
		mustParse(t, "1.13.1"),
	}
	got, err := ComputePath(mustParse(t, "v1.10.3"), mustParse(t, "v1.13.3"), catalog)
	if err != nil {
		t.Fatal(err)
	}
	want := []Version{mustParse(t, "1.11.6"), mustParse(t, "1.12.8"), mustParse(t, "1.13.3")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ComputePath = %v, want %v", got, want)
	}
}

func TestComputePathAlreadyCurrent(t *testing.T) {
	got, err := ComputePath(mustParse(t, "v1.13.3"), mustParse(t, "v1.13.3"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty path, got %v", got)
	}
	// current > target also yields empty.
	got, err = ComputePath(mustParse(t, "v1.13.5"), mustParse(t, "v1.13.3"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty path for current>target, got %v", got)
	}
}

func TestComputePathSingleStep(t *testing.T) {
	// Same minor, just a patch bump: no intermediate minors needed.
	got, err := ComputePath(mustParse(t, "v1.13.1"), mustParse(t, "v1.13.3"), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []Version{mustParse(t, "1.13.3")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ComputePath = %v, want %v", got, want)
	}

	// Adjacent minor jump: single final step, no intermediates.
	got, err = ComputePath(mustParse(t, "v1.12.0"), mustParse(t, "v1.13.3"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ComputePath adjacent = %v, want %v", got, want)
	}
}

func TestComputePathMissingIntermediate(t *testing.T) {
	catalog := []Version{
		mustParse(t, "1.11.6"),
		// 1.12.x missing
		mustParse(t, "1.13.3"),
	}
	_, err := ComputePath(mustParse(t, "v1.10.3"), mustParse(t, "v1.13.3"), catalog)
	if err == nil {
		t.Fatal("expected error for missing intermediate minor")
	}
}

func TestOrderNodesWorkerFirstStable(t *testing.T) {
	in := []NodeRef{
		{Name: "dell01", Role: "controlplane"},
		{Name: "tp4", Role: "worker"},
		{Name: "tp1", Role: "worker"},
		{Name: "cp-a", Role: "controlplane"},
	}
	got := OrderNodes(in)
	wantNames := []string{"tp1", "tp4", "cp-a", "dell01"}
	for i, n := range got {
		if n.Name != wantNames[i] {
			t.Errorf("OrderNodes[%d] = %q, want %q (full: %v)", i, n.Name, wantNames[i], got)
		}
	}
	// Input must not be mutated.
	if in[0].Name != "dell01" {
		t.Errorf("input slice was mutated: %v", in)
	}
}
