package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validYAML = `
cluster:
  name: talos-default
  endpoint: https://192.168.68.100:6443
  talos_version: v1.10.3
  schematic_id: 4a0d65c669d46663f377e7161e50cfd570c401f26fd9e7bda34a0216b6f1922b
secrets:
  backend: onepassword
  onepassword:
    account: my.1password.com
    vault: kubernetes
nodes:
  dell01:
    mac: "d0:94:66:d9:eb:a5"
    ip: 192.168.68.100
    role: controlplane
    arch: amd64
    install_disk: /dev/nvme0n1
    template: dell01.yaml
`

func writeYAML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadValid(t *testing.T) {
	cfg, err := Load(writeYAML(t, validYAML))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cluster.Name != "talos-default" {
		t.Errorf("cluster.name = %q", cfg.Cluster.Name)
	}
	n, ok := cfg.Nodes["dell01"]
	if !ok {
		t.Fatal("dell01 missing")
	}
	if n.MAC != "d0:94:66:d9:eb:a5" {
		t.Errorf("MAC = %q", n.MAC)
	}
	if n.MACHyphen() != "d0-94-66-d9-eb-a5" {
		t.Errorf("MACHyphen = %q", n.MACHyphen())
	}
}

func TestUppercaseMACNormalized(t *testing.T) {
	body := strings.Replace(validYAML, "d0:94:66:d9:eb:a5", "D0:94:66:D9:EB:A5", 1)
	cfg, err := Load(writeYAML(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Nodes["dell01"].MAC != "d0:94:66:d9:eb:a5" {
		t.Errorf("MAC = %q; want lowercase", cfg.Nodes["dell01"].MAC)
	}
}

func TestInvalidMAC(t *testing.T) {
	body := strings.Replace(validYAML, `"d0:94:66:d9:eb:a5"`, `"not-a-mac"`, 1)
	_, err := Load(writeYAML(t, body))
	if err == nil || !strings.Contains(err.Error(), "MAC") {
		t.Fatalf("want MAC error, got %v", err)
	}
}

func TestDuplicateMAC(t *testing.T) {
	body := validYAML + `
  dell02:
    mac: "d0:94:66:d9:eb:a5"
    ip: 192.168.68.101
    role: worker
    arch: amd64
    install_disk: /dev/nvme0n1
    template: dell02.yaml
`
	_, err := Load(writeYAML(t, body))
	if err == nil || !strings.Contains(err.Error(), "duplicate MAC") {
		t.Fatalf("want duplicate MAC error, got %v", err)
	}
	if !strings.Contains(err.Error(), "dell01") || !strings.Contains(err.Error(), "dell02") {
		t.Errorf("err missing node names: %v", err)
	}
}

func TestMissingRequired(t *testing.T) {
	body := strings.Replace(validYAML, "name: talos-default\n", "", 1)
	_, err := Load(writeYAML(t, body))
	if err == nil {
		t.Fatal("expected validation failure")
	}
}

func TestOnepasswordWithoutBlock(t *testing.T) {
	body := strings.Replace(
		validYAML,
		"backend: onepassword\n  onepassword:\n    account: my.1password.com\n    vault: kubernetes\n",
		"backend: onepassword\n",
		1,
	)
	_, err := Load(writeYAML(t, body))
	if err == nil || !strings.Contains(err.Error(), "onepassword block") {
		t.Fatalf("want onepassword block error, got %v", err)
	}
}

func TestInvalidNodeName(t *testing.T) {
	body := strings.Replace(validYAML, "dell01:", "Dell01!:", 1)
	_, err := Load(writeYAML(t, body))
	if err == nil || !strings.Contains(err.Error(), "invalid node name") {
		t.Fatalf("want invalid node name error, got %v", err)
	}
}

func TestHTTPEndpointRejected(t *testing.T) {
	body := strings.Replace(validYAML, "https://192.168.68.100", "http://192.168.68.100", 1)
	_, err := Load(writeYAML(t, body))
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("want https rule error, got %v", err)
	}
}

func TestMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEmptyFile(t *testing.T) {
	_, err := Load(writeYAML(t, ""))
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("want empty error, got %v", err)
	}
}
