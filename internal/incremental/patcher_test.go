package incremental_test

import (
	"testing"

	"github.com/yourusername/mimir/internal/incremental"
)

func TestPlanPatch(t *testing.T) {
	changed := []incremental.ChangedFile{
		{Path: "/src/a.ts", Status: incremental.StatusAdded},
		{Path: "/src/b.ts", Status: incremental.StatusModified},
		{Path: "/src/c.ts", Status: incremental.StatusModified},
		{Path: "/src/d.ts", Status: incremental.StatusDeleted},
	}

	plan := incremental.PlanPatch(changed)

	if len(plan.ToAdd) != 1 {
		t.Errorf("ToAdd count = %d, want 1", len(plan.ToAdd))
	}
	if len(plan.ToUpdate) != 2 {
		t.Errorf("ToUpdate count = %d, want 2", len(plan.ToUpdate))
	}
	if len(plan.ToDelete) != 1 {
		t.Errorf("ToDelete count = %d, want 1", len(plan.ToDelete))
	}
}

func TestPlanPatch_Empty(t *testing.T) {
	plan := incremental.PlanPatch(nil)
	if len(plan.ToAdd)+len(plan.ToUpdate)+len(plan.ToDelete) != 0 {
		t.Error("expected empty plan for nil input")
	}
}
