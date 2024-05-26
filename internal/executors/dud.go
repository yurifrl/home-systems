package executors

import (
	"bytes"
	"strings"

	"github.com/k0kubun/pp/v3"
)

type DudExecutor struct {
}

func (DudExecutor) ExecuteCommand(name string, args ...string) (bytes.Buffer, error) {
	var out bytes.Buffer

	pp.Printf("Executing command: `%s %s`\n", name, strings.Join(args, " "))
	return out, nil
}
