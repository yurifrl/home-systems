package provisionertest_test

import (
	"testing"

	"github.com/yurifrl/nostos/internal/provisioner"
	"github.com/yurifrl/nostos/internal/provisioner/provisionertest"
)

// nilFactory exercises the harness's "skip if not yet wired" branch,
// proving the public signature compiles and runs.
func TestRunComplianceSuiteHarnessCompiles(t *testing.T) {
	provisionertest.RunComplianceSuite(t, func() provisioner.Provisioner {
		return nil
	})
}
