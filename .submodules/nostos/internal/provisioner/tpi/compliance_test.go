package tpi

import (
	"testing"
	"time"

	"github.com/yurifrl/nostos/internal/clockx"
	"github.com/yurifrl/nostos/internal/execx/execxtest"
	"github.com/yurifrl/nostos/internal/provisioner"
	"github.com/yurifrl/nostos/internal/provisioner/provisionertest"
)

func TestTPICompliance(t *testing.T) {
	provisionertest.RunComplianceSuite(t, func() provisioner.Provisioner {
		return New(provisioner.Deps{
			Cmd:   execxtest.New(),
			Clock: clockx.NewFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
		})
	})
}
