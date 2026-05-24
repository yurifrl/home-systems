package execx_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yurifrl/nostos/internal/execx"
	"github.com/yurifrl/nostos/internal/execx/execxtest"
)

func TestFakeCommanderCapturesArgvEnvStdinAndScriptedExit(t *testing.T) {
	want := errors.New("boom")
	fc := execxtest.New(execxtest.Script{Stdout: []byte("hello"), Err: want})

	var stdout bytes.Buffer
	err := fc.Run(context.Background(), "tpi",
		[]string{"--host", "10.0.0.1", "power", "off", "-n", "1"},
		[]string{"TPI_USERNAME=u", "TPI_PASSWORD=p"},
		strings.NewReader("payload"),
		&stdout, nil)
	if !errors.Is(err, want) {
		t.Fatalf("err=%v want %v", err, want)
	}
	if stdout.String() != "hello" {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if len(fc.Calls) != 1 {
		t.Fatalf("calls=%d", len(fc.Calls))
	}
	c := fc.Calls[0]
	if c.Name != "tpi" {
		t.Errorf("name=%q", c.Name)
	}
	if c.Args[0] != "--host" || c.Args[5] != "1" {
		t.Errorf("args=%v", c.Args)
	}
	if c.Env[1] != "TPI_PASSWORD=p" {
		t.Errorf("env=%v", c.Env)
	}
	if string(c.Stdin) != "payload" {
		t.Errorf("stdin=%q", c.Stdin)
	}
}

func TestOSCommanderRunsEcho(t *testing.T) {
	var out bytes.Buffer
	err := execx.OSCommander{}.Run(context.Background(), "/bin/echo",
		[]string{"hi"}, nil, nil, &out, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.TrimSpace(out.String()) != "hi" {
		t.Fatalf("got %q", out.String())
	}
}
