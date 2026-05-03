// Package cluster wraps talosctl bootstrap/status/kubeconfig + native admin cert regen.
package cluster

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
)

// Wipe queue: persisted to state/pending-wipes.json. Consumed by pxe serve.

// QueueWipe marks a MAC for one-shot wipe on next PXE boot.
func QueueWipe(path, mac string) error {
	data := loadWipes(path)
	data[strings.ToLower(mac)] = true
	return saveWipes(path, data)
}

// ConsumeWipe removes a MAC from the wipe queue. Returns true if it was pending.
func ConsumeWipe(path, mac string) (bool, error) {
	data := loadWipes(path)
	_, had := data[strings.ToLower(mac)]
	delete(data, strings.ToLower(mac))
	return had, saveWipes(path, data)
}

// PendingWipes returns the sorted list of MACs currently queued for wipe.
func PendingWipes(path string) []string {
	data := loadWipes(path)
	out := make([]string, 0, len(data))
	for mac := range data {
		out = append(out, mac)
	}
	sort.Strings(out)
	return out
}

// WipeAny returns whether at least one wipe is queued.
func WipeAny(path string) bool {
	return len(loadWipes(path)) > 0
}

func loadWipes(path string) map[string]bool {
	out := map[string]bool{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

func saveWipes(path string, data map[string]bool) error {
	if len(data) == 0 {
		return os.WriteFile(path, []byte("{}\n"), 0o600)
	}
	enc, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, enc, 0o600)
}
