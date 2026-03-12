package graph

import (
	"testing"
	"time"

	"github.com/featherci/featherci/internal/models"
)

func makeStep(name string, deps ...string) *models.BuildStep {
	s := &models.BuildStep{
		ID:     0,
		Name:   name,
		Status: models.StepStatusSuccess,
	}
	if len(deps) > 0 {
		s.DependsOn = deps
	}
	return s
}

func TestCalculate_NoDependencies(t *testing.T) {
	steps := []*models.BuildStep{makeStep("a"), makeStep("b"), makeStep("c")}
	layout := Calculate(steps)
	if layout != nil {
		t.Error("expected nil layout for steps without dependencies")
	}
}

func TestCalculate_SingleStep(t *testing.T) {
	steps := []*models.BuildStep{makeStep("a")}
	layout := Calculate(steps)
	if layout != nil {
		t.Error("expected nil layout for single step")
	}
}

func TestCalculate_LinearChain(t *testing.T) {
	// A → B → C
	steps := []*models.BuildStep{
		makeStep("a"),
		makeStep("b", "a"),
		makeStep("c", "b"),
	}
	layout := Calculate(steps)
	if layout == nil {
		t.Fatal("expected non-nil layout")
	}

	if len(layout.Groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(layout.Groups))
	}

	// Each group should have 1 node
	for i, g := range layout.Groups {
		if len(g.Nodes) != 1 {
			t.Errorf("group %d: expected 1 node, got %d", i, len(g.Nodes))
		}
		if g.Column != i {
			t.Errorf("group %d: expected column %d, got %d", i, i, g.Column)
		}
	}

	// Should have 2 edges: A→B and B→C (each is a separate group-to-group edge)
	if len(layout.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(layout.Edges))
	}

	// Groups should be left-to-right
	for i := 1; i < len(layout.Groups); i++ {
		if layout.Groups[i].X <= layout.Groups[i-1].X {
			t.Errorf("group %d X (%d) should be > group %d X (%d)",
				i, layout.Groups[i].X, i-1, layout.Groups[i-1].X)
		}
	}
}

func TestCalculate_FanOut(t *testing.T) {
	// A → B, A → C (B and C have same deps ["a"] → same group)
	steps := []*models.BuildStep{
		makeStep("a"),
		makeStep("b", "a"),
		makeStep("c", "a"),
	}
	layout := Calculate(steps)
	if layout == nil {
		t.Fatal("expected non-nil layout")
	}

	if len(layout.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(layout.Groups))
	}

	// Group 0: [A], Group 1: [B, C]
	if len(layout.Groups[0].Nodes) != 1 {
		t.Errorf("group 0: expected 1 node, got %d", len(layout.Groups[0].Nodes))
	}
	if len(layout.Groups[1].Nodes) != 2 {
		t.Errorf("group 1: expected 2 nodes, got %d", len(layout.Groups[1].Nodes))
	}

	// 1 edge: group0 → group1 (same dep set, collapsed to one edge)
	if len(layout.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(layout.Edges))
	}
}

func TestCalculate_FanIn(t *testing.T) {
	// A, B → C (A and B have same deps [] → same group)
	steps := []*models.BuildStep{
		makeStep("a"),
		makeStep("b"),
		makeStep("c", "a", "b"),
	}
	layout := Calculate(steps)
	if layout == nil {
		t.Fatal("expected non-nil layout")
	}

	if len(layout.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(layout.Groups))
	}

	// Group 0: [A, B], Group 1: [C]
	if len(layout.Groups[0].Nodes) != 2 {
		t.Errorf("group 0: expected 2 nodes, got %d", len(layout.Groups[0].Nodes))
	}
	if len(layout.Groups[1].Nodes) != 1 {
		t.Errorf("group 1: expected 1 node, got %d", len(layout.Groups[1].Nodes))
	}

	// 1 edge: group0 → group1 (A,B in same source group)
	if len(layout.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(layout.Edges))
	}
}

func TestCalculate_Diamond(t *testing.T) {
	// A → B, A → C, B → D, C → D
	// B and C share deps ["a"] → same group
	steps := []*models.BuildStep{
		makeStep("a"),
		makeStep("b", "a"),
		makeStep("c", "a"),
		makeStep("d", "b", "c"),
	}
	layout := Calculate(steps)
	if layout == nil {
		t.Fatal("expected non-nil layout")
	}

	if len(layout.Groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(layout.Groups))
	}

	// Column 0: [A], Column 1: [B, C], Column 2: [D]
	if len(layout.Groups[0].Nodes) != 1 {
		t.Errorf("group 0: expected 1 node, got %d", len(layout.Groups[0].Nodes))
	}
	if len(layout.Groups[1].Nodes) != 2 {
		t.Errorf("group 1: expected 2 nodes, got %d", len(layout.Groups[1].Nodes))
	}
	if len(layout.Groups[2].Nodes) != 1 {
		t.Errorf("group 2: expected 1 node, got %d", len(layout.Groups[2].Nodes))
	}

	// 2 edges: group0→group1, group1→group2
	if len(layout.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(layout.Edges))
	}
}

func TestCalculate_MixedDepsAndNoDeps(t *testing.T) {
	// A has no deps, B depends on A, C has no deps but is separate
	// All with deps: B depends on A. So hasDeps = true.
	steps := []*models.BuildStep{
		makeStep("a"),
		makeStep("b", "a"),
		makeStep("c"),
	}
	layout := Calculate(steps)
	if layout == nil {
		t.Fatal("expected non-nil layout")
	}

	// Column 0: [A, C] (same deps: none), Column 1: [B]
	if len(layout.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(layout.Groups))
	}
	if len(layout.Groups[0].Nodes) != 2 {
		t.Errorf("group 0: expected 2 nodes, got %d", len(layout.Groups[0].Nodes))
	}
	if len(layout.Groups[1].Nodes) != 1 {
		t.Errorf("group 1: expected 1 node, got %d", len(layout.Groups[1].Nodes))
	}
}

func TestCalculate_Dimensions(t *testing.T) {
	steps := []*models.BuildStep{
		makeStep("a"),
		makeStep("b", "a"),
	}
	layout := Calculate(steps)
	if layout == nil {
		t.Fatal("expected non-nil layout")
	}

	if layout.Width <= 0 {
		t.Errorf("expected positive width, got %d", layout.Width)
	}
	if layout.Height <= 0 {
		t.Errorf("expected positive height, got %d", layout.Height)
	}

	// Each group should have valid dimensions
	for i, g := range layout.Groups {
		if g.Width <= 0 || g.Height <= 0 {
			t.Errorf("group %d: invalid dimensions %dx%d", i, g.Width, g.Height)
		}
	}
}

func TestCalculate_ConnectionPoints(t *testing.T) {
	steps := []*models.BuildStep{
		makeStep("a"),
		makeStep("b", "a"),
	}
	layout := Calculate(steps)
	if layout == nil {
		t.Fatal("expected non-nil layout")
	}

	for i, g := range layout.Groups {
		// LeftX should be at group X
		if g.LeftX != g.X {
			t.Errorf("group %d: LeftX %d != X %d", i, g.LeftX, g.X)
		}
		// RightX should be at group X + Width
		if g.RightX != g.X+g.Width {
			t.Errorf("group %d: RightX %d != X+Width %d", i, g.RightX, g.X+g.Width)
		}
		// LeftY and RightY should be vertical center of group
		expectedY := g.Y + g.Height/2
		if g.LeftY != expectedY {
			t.Errorf("group %d: LeftY %d != center %d", i, g.LeftY, expectedY)
		}
		if g.RightY != expectedY {
			t.Errorf("group %d: RightY %d != center %d", i, g.RightY, expectedY)
		}
	}
}

func TestCalculate_MixedDepsPartialGroup(t *testing.T) {
	// Col 0: [build-docker, simple] — same deps (none), one group
	// Col 1: [flipper] depends on simple only — one group
	//         [deploy-to-staging] depends on both — separate group
	steps := []*models.BuildStep{
		makeStep("build-docker"),
		makeStep("simple"),
		makeStep("deploy-to-staging", "build-docker", "simple"),
		makeStep("flipper", "simple"),
	}
	layout := Calculate(steps)
	if layout == nil {
		t.Fatal("expected non-nil layout")
	}

	// 3 groups: col0=[build-docker,simple], col1=[flipper], col1=[deploy-to-staging]
	if len(layout.Groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(layout.Groups))
	}

	// 2 edges: col0-group → flipper-group, col0-group → deploy-group
	if len(layout.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(layout.Edges))
	}

	// Verify edges target different Y positions (flipper and deploy-to-staging are in different groups)
	edgeYs := make(map[int]bool)
	for _, e := range layout.Edges {
		edgeYs[e.ToY] = true
	}
	if len(edgeYs) < 2 {
		t.Error("expected edges to target different Y positions for different groups")
	}
}

func TestEdgePath_Straight(t *testing.T) {
	e := Edge{FromX: 100, FromY: 50, ToX: 200, ToY: 50}
	path := EdgePath(e)
	expected := "M 100 50 L 200 50"
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestEdgePath_Curved(t *testing.T) {
	e := Edge{FromX: 100, FromY: 50, ToX: 200, ToY: 100}
	path := EdgePath(e)
	expected := "M 100 50 C 150 50 150 100 200 100"
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestCalculate_NodeDuration(t *testing.T) {
	now := time.Now()
	started := now.Add(-5 * time.Minute)
	finished := now.Add(-2 * time.Minute)

	steps := []*models.BuildStep{
		{
			Name:       "a",
			Status:     models.StepStatusSuccess,
			StartedAt:  &started,
			FinishedAt: &finished,
		},
		{
			Name:      "b",
			Status:    models.StepStatusRunning,
			DependsOn: []string{"a"},
		},
	}
	layout := Calculate(steps)
	if layout == nil {
		t.Fatal("expected non-nil layout")
	}

	// First group, first node should have a duration string
	if layout.Groups[0].Nodes[0].Duration == "" {
		t.Error("expected non-empty duration for completed step")
	}
	// Second group, first node has no StartedAt so empty duration
	if layout.Groups[1].Nodes[0].Duration != "" {
		t.Error("expected empty duration for step without StartedAt")
	}
}
