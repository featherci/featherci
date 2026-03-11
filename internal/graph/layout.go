// Package graph provides pipeline DAG layout calculation for SVG rendering.
package graph

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/featherci/featherci/internal/models"
)

const (
	nodeWidth   = 240
	nodeHeight  = 36
	nodeGapY    = 8
	groupPadX   = 16
	groupPadY   = 12
	colGap      = 60
	subGroupGap = 16
	graphPadX   = 24
	graphPadY   = 24
	dotRadius   = 4
)

// Layout holds the complete graph layout for SVG rendering.
type Layout struct {
	Groups []Group
	Edges  []Edge
	Width  int
	Height int
}

// Group is a set of nodes sharing the same column and dependency set.
type Group struct {
	Nodes          []Node
	X, Y           int
	Width          int
	Height         int
	Column         int
	LeftX, LeftY   int  // center-left connection point
	RightX, RightY int  // center-right connection point
	HasIncoming    bool // true if any edge targets this group
	HasOutgoing    bool // true if any edge originates from this group
}

// Node is a single build step within a group.
type Node struct {
	ID               int64
	Name             string
	Status           string
	Duration         string
	RequiresApproval bool
	Approved         bool // true if ApprovedBy is set (step was approved)
	X, Y             int  // absolute position within SVG
}

// Edge connects the right side of one group to the left side of another.
type Edge struct {
	FromX, FromY int
	ToX, ToY     int
}

// groupEntry pairs a Group with its dependency set key for layout ordering.
type groupEntry struct {
	group  Group
	depKey string
}

// Calculate computes the pipeline graph layout from build steps.
// Returns nil if no steps have dependencies (flat pipeline).
func Calculate(steps []*models.BuildStep) *Layout {
	if len(steps) <= 1 {
		return nil
	}

	// Check if any step has dependencies
	hasDeps := false
	for _, s := range steps {
		if len(s.DependsOn) > 0 {
			hasDeps = true
			break
		}
	}
	if !hasDeps {
		return nil
	}

	// Build name→step map
	byName := make(map[string]*models.BuildStep, len(steps))
	for _, s := range steps {
		byName[s.Name] = s
	}

	// Assign columns via BFS: no deps → column 0; others → max(dep columns) + 1
	columns := make(map[string]int, len(steps))
	for _, s := range steps {
		assignColumn(s.Name, byName, columns)
	}

	// Sub-group steps by (column, sorted dependency set).
	// Steps with identical deps share a visual box.
	type sgKey struct {
		col    int
		depKey string
	}

	maxCol := 0
	sgSteps := make(map[sgKey][]*models.BuildStep)
	for _, s := range steps {
		col := columns[s.Name]
		key := sgKey{col, depSetKey(s)}
		sgSteps[key] = append(sgSteps[key], s)
		if col > maxCol {
			maxCol = col
		}
	}

	// Collect sub-group keys per column, sort steps within each alphabetically
	colKeys := make(map[int][]sgKey)
	for key := range sgSteps {
		ss := sgSteps[key]
		sort.Slice(ss, func(i, j int) bool { return ss[i].Name < ss[j].Name })
		colKeys[key.col] = append(colKeys[key.col], key)
	}
	// Stable initial sort of sub-group keys by depKey
	for col := range colKeys {
		keys := colKeys[col]
		sort.Slice(keys, func(i, j int) bool { return keys[i].depKey < keys[j].depKey })
	}

	// Build Group objects (without X/Y positions yet)
	groupWidth := nodeWidth + 2*groupPadX
	colGroupIndices := make(map[int][]int) // col → indices into allGroups
	var allGroups []groupEntry
	colHeights := make(map[int]int)

	for col := 0; col <= maxCol; col++ {
		keys := colKeys[col]
		if len(keys) == 0 {
			continue
		}
		totalH := 0
		for i, key := range keys {
			ss := sgSteps[key]
			h := 2*groupPadY + len(ss)*nodeHeight + (len(ss)-1)*nodeGapY
			g := Group{Column: col, Width: groupWidth, Height: h}
			for _, s := range ss {
				g.Nodes = append(g.Nodes, Node{
					ID:               s.ID,
					Name:             truncate(s.Name, 24),
					Status:           string(s.Status),
					Duration:         formatStepDuration(s),
					RequiresApproval: s.RequiresApproval,
					Approved:         s.ApprovedBy != nil,
				})
			}
			idx := len(allGroups)
			allGroups = append(allGroups, groupEntry{group: g, depKey: key.depKey})
			colGroupIndices[col] = append(colGroupIndices[col], idx)
			totalH += h
			if i > 0 {
				totalH += subGroupGap
			}
		}
		colHeights[col] = totalH
	}

	// Find max column height for vertical centering
	maxHeight := 0
	for _, h := range colHeights {
		if h > maxHeight {
			maxHeight = h
		}
	}
	totalHeight := maxHeight + 2*graphPadY

	// Position groups left-to-right; order sub-groups by avg source Y to minimize crossings
	// Map step name → group index for source-Y lookups
	nameToGroupIdx := make(map[string]int, len(steps))
	x := graphPadX

	for col := 0; col <= maxCol; col++ {
		indices := colGroupIndices[col]
		if len(indices) == 0 {
			continue
		}

		// For col > 0, sort sub-groups by average Y of their source groups
		if col > 0 {
			sort.SliceStable(indices, func(i, j int) bool {
				return avgSourceY(allGroups[indices[i]].depKey, nameToGroupIdx, allGroups) <
					avgSourceY(allGroups[indices[j]].depKey, nameToGroupIdx, allGroups)
			})
		}

		// Stack sub-groups vertically, centered in the column
		colH := colHeights[col]
		y := graphPadY + (maxHeight-colH)/2

		for gi, idx := range indices {
			if gi > 0 {
				y += subGroupGap
			}
			g := &allGroups[idx].group
			g.X = x
			g.Y = y

			for j := range g.Nodes {
				g.Nodes[j].X = g.X + groupPadX
				g.Nodes[j].Y = g.Y + groupPadY + j*(nodeHeight+nodeGapY)
				nameToGroupIdx[g.Nodes[j].Name] = idx
			}

			g.LeftX = g.X
			g.LeftY = g.Y + g.Height/2
			g.RightX = g.X + g.Width
			g.RightY = g.Y + g.Height/2

			y += g.Height
		}

		x += groupWidth + colGap
	}

	totalWidth := x - colGap + graphPadX

	// Collect groups into final slice
	groups := make([]Group, len(allGroups))
	for i := range allGroups {
		groups[i] = allGroups[i].group
	}

	// Build group-to-group edges (one edge per source-group → target-group pair).
	// Since groups contain steps with identical deps, this is the right granularity.
	// nameToGroupIdx was populated during positioning above.
	type edgeKey struct{ from, to int }
	seenEdges := make(map[edgeKey]bool)

	var edges []Edge
	for _, s := range steps {
		if len(s.DependsOn) == 0 {
			continue
		}
		toIdx, ok := nameToGroupIdx[truncate(s.Name, 24)]
		if !ok {
			continue
		}

		for _, depName := range s.DependsOn {
			fromIdx, ok := nameToGroupIdx[truncate(depName, 24)]
			if !ok {
				continue
			}

			key := edgeKey{fromIdx, toIdx}
			if seenEdges[key] {
				continue
			}
			seenEdges[key] = true

			fromGroup := &groups[fromIdx]
			toGroup := &groups[toIdx]
			fromGroup.HasOutgoing = true
			toGroup.HasIncoming = true

			edges = append(edges, Edge{
				FromX: fromGroup.RightX,
				FromY: fromGroup.RightY,
				ToX:   toGroup.LeftX,
				ToY:   toGroup.LeftY,
			})
		}
	}

	return &Layout{
		Groups: groups,
		Edges:  edges,
		Width:  totalWidth,
		Height: totalHeight,
	}
}

// depSetKey returns a canonical string key for a step's sorted dependency set.
func depSetKey(s *models.BuildStep) string {
	if len(s.DependsOn) == 0 {
		return ""
	}
	sorted := make([]string, len(s.DependsOn))
	copy(sorted, s.DependsOn)
	sort.Strings(sorted)
	return strings.Join(sorted, "\x00")
}

// avgSourceY computes the average center Y of source groups for a dependency set key.
func avgSourceY(dk string, nameToGroupIdx map[string]int, allGroups []groupEntry) float64 {
	if dk == "" {
		return 0
	}
	deps := strings.Split(dk, "\x00")
	sum := 0.0
	count := 0
	seen := make(map[int]bool)
	for _, dep := range deps {
		idx, ok := nameToGroupIdx[truncate(dep, 24)]
		if !ok || seen[idx] {
			continue
		}
		seen[idx] = true
		g := &allGroups[idx].group
		sum += float64(g.LeftY)
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

// assignColumn recursively assigns a column to a step based on dependencies.
func assignColumn(name string, byName map[string]*models.BuildStep, columns map[string]int) int {
	if col, ok := columns[name]; ok {
		return col
	}

	s, ok := byName[name]
	if !ok {
		columns[name] = 0
		return 0
	}

	if len(s.DependsOn) == 0 {
		columns[name] = 0
		return 0
	}

	maxDepCol := 0
	for _, dep := range s.DependsOn {
		depCol := assignColumn(dep, byName, columns)
		if depCol > maxDepCol {
			maxDepCol = depCol
		}
	}

	col := maxDepCol + 1
	columns[name] = col
	return col
}

// EdgePath returns an SVG path string for an edge.
func EdgePath(e Edge) string {
	if e.FromY == e.ToY {
		return fmt.Sprintf("M %d %d L %d %d", e.FromX, e.FromY, e.ToX, e.ToY)
	}
	midX := (e.FromX + e.ToX) / 2
	return fmt.Sprintf("M %d %d C %d %d %d %d %d %d",
		e.FromX, e.FromY,
		midX, e.FromY,
		midX, e.ToY,
		e.ToX, e.ToY)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func formatStepDuration(s *models.BuildStep) string {
	if s.StartedAt == nil {
		return ""
	}
	var d time.Duration
	if s.FinishedAt != nil {
		d = s.FinishedAt.Sub(*s.StartedAt)
	} else {
		d = time.Since(*s.StartedAt)
	}

	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		if secs == 0 {
			return fmt.Sprintf("%dm", mins)
		}
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", hours, mins)
}
