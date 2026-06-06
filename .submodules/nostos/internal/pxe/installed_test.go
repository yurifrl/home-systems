package pxe

import (
	"path/filepath"
	"testing"
)

// TestInstalledRoundTrip proves MarkInstalled/IsInstalled/ClearInstalled
// persist and clear MAC state, tolerate a missing file, and normalize MAC
// keys to lowercase (mirrors cluster/wipe.go semantics).
func TestInstalledRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installed-macs.json")

	// Missing file: nothing is installed, no error.
	if IsInstalled(path, "d0-94-66-d9-eb-a5") {
		t.Fatal("IsInstalled on missing file should be false")
	}

	if err := MarkInstalled(path, "d0-94-66-d9-eb-a5"); err != nil {
		t.Fatalf("MarkInstalled: %v", err)
	}
	if !IsInstalled(path, "d0-94-66-d9-eb-a5") {
		t.Fatal("expected MAC installed after MarkInstalled")
	}

	if err := ClearInstalled(path, "d0-94-66-d9-eb-a5"); err != nil {
		t.Fatalf("ClearInstalled: %v", err)
	}
	if IsInstalled(path, "d0-94-66-d9-eb-a5") {
		t.Fatal("expected MAC not installed after ClearInstalled")
	}
}

// TestInstalledLowercaseNormalization proves keys are normalized: an
// uppercase MAC written is found via its lowercase form and vice versa.
func TestInstalledLowercaseNormalization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installed-macs.json")

	if err := MarkInstalled(path, "D0-94-66-D9-EB-A5"); err != nil {
		t.Fatalf("MarkInstalled: %v", err)
	}
	if !IsInstalled(path, "d0-94-66-d9-eb-a5") {
		t.Fatal("uppercase MarkInstalled not found via lowercase IsInstalled")
	}
	if err := ClearInstalled(path, "d0-94-66-d9-eb-a5"); err != nil {
		t.Fatalf("ClearInstalled: %v", err)
	}
	if IsInstalled(path, "D0-94-66-D9-EB-A5"); IsInstalled(path, "D0-94-66-D9-EB-A5") {
		t.Fatal("lowercase ClearInstalled did not clear uppercase MAC")
	}
}

// TestClearInstalledMissingFile proves ClearInstalled on a missing file is a
// no-op success (tolerant of absent state).
func TestClearInstalledMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installed-macs.json")
	if err := ClearInstalled(path, "d0-94-66-d9-eb-a5"); err != nil {
		t.Fatalf("ClearInstalled on missing file: %v", err)
	}
}
