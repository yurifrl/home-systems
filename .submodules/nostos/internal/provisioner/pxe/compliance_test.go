package pxe

import (
	"testing"

	"github.com/yurifrl/nostos/internal/clockx"
	"github.com/yurifrl/nostos/internal/execx/execxtest"
	"github.com/yurifrl/nostos/internal/provisioner"
	"github.com/yurifrl/nostos/internal/provisioner/provisionertest"
)

func TestPXECompliance(t *testing.T) {
	provisionertest.RunComplianceSuite(t, func() provisioner.Provisioner {
		return New(provisioner.Deps{
			Cmd:   execxtest.New(),
			Clock: clockx.NewFakeClock(timeZero()),
		})
	})
}
