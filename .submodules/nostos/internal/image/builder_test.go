package image

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRawImage writes a small placeholder file that mimics a Talos raw image
// for builder tests. The bytes are arbitrary — only the round-trip + sidecar
// emission is being validated, not the Talos contents.
func fakeRawImage(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "metal-arm64.raw")
	if err := os.WriteFile(path, []byte("FAKE-TALOS-IMAGE-CONTENTS"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuilderValidate(t *testing.T) {
	dir := t.TempDir()
	placeholder := filepath.Join(dir, "placeholder")
	if err := os.WriteFile(placeholder, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		b       Builder
		wantErr string
	}{
		{
			name:    "missing NodeName",
			b:       Builder{NodeArch: "arm64", RawImagePath: "x", OutPath: "y"},
			wantErr: "NodeName is empty",
		},
		{
			name:    "bad arch",
			b:       Builder{NodeName: "rpi01", NodeArch: "ppc64", RawImagePath: "x", OutPath: "y"},
			wantErr: "unsupported arch",
		},
		{
			name:    "raw missing",
			b:       Builder{NodeName: "rpi01", NodeArch: "arm64", RawImagePath: "/nonexistent/raw", OutPath: "y"},
			wantErr: "raw image not found",
		},
		{
			name:    "compress + device conflict",
			b:       Builder{NodeName: "rpi01", NodeArch: "arm64", RawImagePath: placeholder, OutPath: "/dev/null", Out: ModeDevice, Compress: true},
			wantErr: "incompatible with --device",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.b.validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("got %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestBuilderAssembleFile(t *testing.T) {
	dir := t.TempDir()
	src := fakeRawImage(t, dir)
	out := filepath.Join(dir, "rpi01.raw")
	cfg := []byte("# fake machineconfig\nversion: v1alpha1\n")

	b := &Builder{
		NodeName:      "rpi01",
		NodeArch:      "arm64",
		RawImagePath:  src,
		MachineConfig: cfg,
		Out:           ModeFile,
		OutPath:       out,
	}
	res, err := b.Assemble(context.Background())
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if res.ImagePath != out {
		t.Errorf("ImagePath: got %q, want %q", res.ImagePath, out)
	}
	if res.ConfigPath != filepath.Join(dir, "rpi01-config.yaml") {
		t.Errorf("ConfigPath: got %q", res.ConfigPath)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(got) != "FAKE-TALOS-IMAGE-CONTENTS" {
		t.Errorf("output bytes: got %q", string(got))
	}
	gotCfg, err := os.ReadFile(res.ConfigPath)
	if err != nil {
		t.Fatalf("read config sidecar: %v", err)
	}
	if string(gotCfg) != string(cfg) {
		t.Errorf("config bytes mismatch")
	}
}

func TestBuilderAssembleCompressed(t *testing.T) {
	dir := t.TempDir()
	src := fakeRawImage(t, dir)
	out := filepath.Join(dir, "rpi01.raw")

	b := &Builder{
		NodeName:     "rpi01",
		NodeArch:     "arm64",
		RawImagePath: src,
		Out:          ModeFile,
		OutPath:      out,
		Compress:     true,
	}
	res, err := b.Assemble(context.Background())
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if res.ImagePath != out+".xz" {
		t.Errorf("ImagePath: got %q, want %q", res.ImagePath, out+".xz")
	}
	if _, err := os.Stat(out + ".xz"); err != nil {
		t.Fatalf("expected compressed output: %v", err)
	}
}

func TestBuilderEEPROMEmission(t *testing.T) {
	dir := t.TempDir()
	src := fakeRawImage(t, dir)
	fwDir := filepath.Join(dir, "fw")
	if err := os.MkdirAll(fwDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"start4.elf", "fixup4.dat", "recovery.bin", "pieeprom.bin"} {
		if err := os.WriteFile(filepath.Join(fwDir, n), []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "rpi01.raw")
	b := &Builder{
		NodeName:       "rpi01",
		NodeArch:       "arm64",
		NodeOverlay:    "rpi_generic",
		RawImagePath:   src,
		MachineConfig:  []byte("cfg"),
		RPiFirmwareDir: fwDir,
		Out:            ModeFile,
		OutPath:        out,
	}
	res, err := b.Assemble(context.Background())
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if res.EEPROMPath == "" {
		t.Fatal("expected EEPROMPath to be populated for rpi_generic node")
	}
	for _, n := range EEPROMFiles {
		p := filepath.Join(res.EEPROMPath, n)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing eeprom file %s: %v", p, err)
		}
	}
	bootConf, err := os.ReadFile(filepath.Join(res.EEPROMPath, "boot.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bootConf), "BOOT_ORDER=0xf21") {
		t.Errorf("boot.conf missing BOOT_ORDER=0xf21: %s", bootConf)
	}
}

func TestSidecarPath(t *testing.T) {
	cases := []struct {
		in, suffix, want string
	}{
		{"rpi01.raw", "-config.yaml", "rpi01-config.yaml"},
		{"rpi01.raw.xz", "-config.yaml", "rpi01-config.yaml"},
		{"path/to/dell01.img", "-eeprom.img", "path/to/dell01-eeprom.img"},
		{"plain", "-config.yaml", "plain-config.yaml"},
	}
	for _, c := range cases {
		got := sidecarPath(c.in, c.suffix)
		if got != c.want {
			t.Errorf("sidecarPath(%q, %q): got %q, want %q", c.in, c.suffix, got, c.want)
		}
	}
}
