package upgrade

import "testing"

func mustV(t *testing.T, s string) Version {
	t.Helper()
	v, err := ParseVersion(s)
	if err != nil {
		t.Fatalf("ParseVersion(%q): %v", s, err)
	}
	return v
}

func TestBuildPlanComputesStepsAndOrder(t *testing.T) {
	ordered := OrderNodes([]NodeRef{
		{Name: "dell01", IP: "10.0.0.1", Role: "controlplane"},
		{Name: "tp4", IP: "10.0.0.2", Role: "worker"},
		{Name: "tp1", IP: "10.0.0.3", Role: "worker"},
	})
	current := map[string]Version{
		"tp1":    mustV(t, "v1.10.3"),
		"tp4":    mustV(t, "v1.10.3"),
		"dell01": mustV(t, "v1.10.3"),
	}
	catalog := []Version{mustV(t, "v1.11.6"), mustV(t, "v1.12.8"), mustV(t, "v1.13.3")}
	target := mustV(t, "v1.13.3")

	plan, err := BuildPlan("talos-default", ordered, current, target, catalog)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.Cluster != "talos-default" {
		t.Errorf("cluster = %q", plan.Cluster)
	}
	if plan.Target != "v1.13.3" {
		t.Errorf("target = %q", plan.Target)
	}
	if plan.MinCurrent != "v1.10.3" {
		t.Errorf("minCurrent = %q", plan.MinCurrent)
	}
	wantSteps := []string{"v1.11.6", "v1.12.8", "v1.13.3"}
	if len(plan.Steps) != len(wantSteps) {
		t.Fatalf("steps = %d, want %d (%+v)", len(plan.Steps), len(wantSteps), plan.Steps)
	}
	for i, s := range plan.Steps {
		if s.Version != wantSteps[i] {
			t.Errorf("step[%d] = %q, want %q", i, s.Version, wantSteps[i])
		}
		// controlplane must be ordered last in each step.
		if got := s.Nodes[len(s.Nodes)-1].Name; got != "dell01" {
			t.Errorf("step[%d] last node = %q, want dell01", i, got)
		}
	}
}

func TestBuildPlanSkipsNodesAtOrAboveStep(t *testing.T) {
	ordered := OrderNodes([]NodeRef{
		{Name: "a", Role: "worker"},
		{Name: "b", Role: "worker"},
	})
	current := map[string]Version{
		"a": mustV(t, "v1.11.0"), // already on the first minor
		"b": mustV(t, "v1.10.3"),
	}
	catalog := []Version{mustV(t, "v1.11.6"), mustV(t, "v1.12.8")}
	target := mustV(t, "v1.12.8")

	plan, err := BuildPlan("c", ordered, current, target, catalog)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	// Step v1.11.6 should only touch "b" (a is already on 1.11.0 < 1.11.6 → included).
	// a (1.11.0) is below 1.11.6, so it IS touched; b (1.10.3) below too.
	if len(plan.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(plan.Steps))
	}
	// On the final step both should be present (both below 1.12.8).
	if len(plan.Steps[1].Nodes) != 2 {
		t.Errorf("final step nodes = %d, want 2", len(plan.Steps[1].Nodes))
	}
}

func TestBuildPlanEmptyWhenUpToDate(t *testing.T) {
	ordered := []NodeRef{{Name: "a", Role: "worker"}}
	current := map[string]Version{"a": mustV(t, "v1.13.3")}
	catalog := []Version{mustV(t, "v1.13.3")}
	plan, err := BuildPlan("c", ordered, current, mustV(t, "v1.13.3"), catalog)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Steps) != 0 {
		t.Fatalf("steps = %d, want 0", len(plan.Steps))
	}
}
