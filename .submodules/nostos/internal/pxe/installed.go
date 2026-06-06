package pxe

import (
	"encoding/json"
	"os"
	"strings"
)

// Installed-state store: persisted to state/installed-macs.json. The SERVE
// side reads it (IsInstalled) to decide whether to hand a MAC the install
// chain or a boot-from-disk script; the install-flow provisioner writes it
// (MarkInstalled on config-fetch, ClearInstalled on (re)install start).
//
// This MIRRORS internal/cluster/wipe.go's map[string]bool-as-JSON pattern but
// lives in package pxe so the SERVE side never imports internal/cluster
// (which would create an import cycle: provisioner/pxe imports both).

// MarkInstalled records that a MAC has fetched its config and committed to
// installing, so a subsequent PXE boot should settle to local disk.
func MarkInstalled(path, mac string) error {
	data := loadInstalled(path)
	data[strings.ToLower(mac)] = true
	return saveInstalled(path, data)
}

// ClearInstalled removes a MAC from the installed set, so the next PXE boot
// serves the install chain again. Called when a (re)install starts.
func ClearInstalled(path, mac string) error {
	data := loadInstalled(path)
	delete(data, strings.ToLower(mac))
	return saveInstalled(path, data)
}

// IsInstalled reports whether the MAC has been marked installed. Tolerates a
// missing file (returns false).
func IsInstalled(path, mac string) bool {
	return loadInstalled(path)[strings.ToLower(mac)]
}

func loadInstalled(path string) map[string]bool {
	out := map[string]bool{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

func saveInstalled(path string, data map[string]bool) error {
	if len(data) == 0 {
		return os.WriteFile(path, []byte("{}\n"), 0o600)
	}
	enc, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, enc, 0o600)
}
