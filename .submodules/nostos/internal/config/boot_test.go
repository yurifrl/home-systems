package config

import (
	"strings"
	"testing"
)

const dell01Only = `
cluster:
  name: talos-default
  endpoint: https://192.168.68.100:6443
  talos_version: v1.10.3
  schematic_id: 4a0d65c669d46663f377e7161e50cfd570c401f26fd9e7bda34a0216b6f1922b
secrets:
  backend: env
nodes:
  dell01:
    mac: "d0:94:66:d9:eb:a5"
    ip: 192.168.68.100
    role: controlplane
    arch: amd64
    install_disk: /dev/nvme0n1
    template: dell01.yaml
`

const tp1tp4 = `
cluster:
  name: talos-default
  endpoint: https://192.168.68.100:6443
  talos_version: v1.10.3
  schematic_id: 4a0d65c669d46663f377e7161e50cfd570c401f26fd9e7bda34a0216b6f1922b
secrets:
  backend: env
nodes:
  tp1:
    mac: "02:00:00:00:00:01"
    ip: 192.168.68.107
    role: worker
    arch: arm64
    install_disk: /dev/mmcblk0
    template: tp.yaml
    boot:
      method: tpi
      tpi:
        host: "192.168.68.10"
        slot: 1
        username_ref: "op://kubernetes/turingpi/username"
        password_ref: "op://kubernetes/turingpi/password"
  tp4:
    mac: "02:00:00:00:00:04"
    ip: 192.168.68.114
    role: worker
    arch: arm64
    install_disk: /dev/mmcblk0
    template: tp.yaml
    boot:
      method: tpi
      tpi:
        host: "192.168.68.10"
        slot: 4
        identity_file_ref: "op://kubernetes/turingpi/ssh_key"
`

func TestLoadDell01OnlyDefaultsToPXE(t *testing.T) {
	cfg, err := Load(writeYAML(t, dell01Only))
	if err != nil {
		t.Fatal(err)
	}
	n := cfg.Nodes["dell01"]
	if n.Boot.Method != "pxe" {
		t.Fatalf("Boot.Method = %q; want pxe", n.Boot.Method)
	}
	if n.Boot.TPI != nil {
		t.Fatalf("Boot.TPI = %+v; want nil", n.Boot.TPI)
	}
}

func TestLoadTP1TP4(t *testing.T) {
	cfg, err := Load(writeYAML(t, tp1tp4))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Nodes["tp1"].Boot.Method; got != "tpi" {
		t.Fatalf("tp1 method = %q", got)
	}
	if got := cfg.Nodes["tp1"].Boot.TPI.Slot; got != 1 {
		t.Fatalf("tp1 slot = %d", got)
	}
	if got := cfg.Nodes["tp4"].Boot.TPI.IdentityFileRef; got == "" {
		t.Fatal("tp4 identity_file_ref empty")
	}
}

func TestDuplicateHostSlotRejected(t *testing.T) {
	body := strings.Replace(tp1tp4, "slot: 4", "slot: 1", 1)
	_, err := Load(writeYAML(t, body))
	if err == nil || !strings.Contains(err.Error(), "duplicate (host, slot)") {
		t.Fatalf("want collision error, got %v", err)
	}
	if !strings.Contains(err.Error(), "tp1") || !strings.Contains(err.Error(), "tp4") {
		t.Errorf("err missing both names: %v", err)
	}
}

func TestTPINoCredsAllowed(t *testing.T) {
	body := strings.Replace(tp1tp4,
		`        username_ref: "op://kubernetes/turingpi/username"
        password_ref: "op://kubernetes/turingpi/password"`,
		"", 1)
	body = strings.Replace(body,
		`        identity_file_ref: "op://kubernetes/turingpi/ssh_key"`,
		"", 1)
	cfg, err := Load(writeYAML(t, body))
	if err != nil {
		t.Fatalf("want creds-less tpi to load, got %v", err)
	}
	if cfg.Nodes["tp1"].Boot.TPI.UsernameRef != "" {
		t.Fatalf("tp1 username_ref leaked: %q", cfg.Nodes["tp1"].Boot.TPI.UsernameRef)
	}
	if cfg.Nodes["tp4"].Boot.TPI.IdentityFileRef != "" {
		t.Fatalf("tp4 identity_file_ref leaked")
	}
}

func TestTPIMACOptional(t *testing.T) {
	body := strings.Replace(tp1tp4,
		`    mac: "02:00:00:00:00:01"
`, "", 1)
	body = strings.Replace(body,
		`    mac: "02:00:00:00:00:04"
`, "", 1)
	if _, err := Load(writeYAML(t, body)); err != nil {
		t.Fatalf("tpi nodes should not require mac, got %v", err)
	}
}

func TestPXERequiresMAC(t *testing.T) {
	body := strings.Replace(dell01Only,
		`    mac: "d0:94:66:d9:eb:a5"
`, "", 1)
	_, err := Load(writeYAML(t, body))
	if err == nil || !strings.Contains(err.Error(), "requires mac") {
		t.Fatalf("want pxe mac requirement error, got %v", err)
	}
}

func TestTPIRequiresCreds(t *testing.T) {
	// With v0.2 relaxation, missing creds is allowed (cached token / prompt).
	body := strings.Replace(tp1tp4,
		`        username_ref: "op://kubernetes/turingpi/username"
        password_ref: "op://kubernetes/turingpi/password"`,
		"", 1)
	if _, err := Load(writeYAML(t, body)); err != nil {
		t.Fatalf("creds optional now, got %v", err)
	}
}

func TestRefRejectsEnvScheme(t *testing.T) {
	body := strings.Replace(tp1tp4,
		`"op://kubernetes/turingpi/username"`,
		`"env://TPI_USER"`, 1)
	_, err := Load(writeYAML(t, body))
	if err == nil || !strings.Contains(err.Error(), "env://") {
		t.Fatalf("want env:// rejection, got %v", err)
	}
}

func TestRefRejectsInlineLiteral(t *testing.T) {
	body := strings.Replace(tp1tp4,
		`"op://kubernetes/turingpi/password"`,
		`"hunter2"`, 1)
	_, err := Load(writeYAML(t, body))
	if err == nil || !strings.Contains(err.Error(), "must start with") {
		t.Fatalf("want literal rejection, got %v", err)
	}
}
