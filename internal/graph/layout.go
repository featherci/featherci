// Package graph provides pipeline DAG layout calculation for SVG rendering.
package graph

import (
	"fmt"
	"sort"
	"time"

	"github.com/featherci/featherci/internal/models"
)

const (
	nodeWidth  = 240
	nodeHeight = 36
	nodeGapY   = 8
	groupPadX  = 16
	groupPadY  = 12
	colGap     = 60
	graphPadX  = 24
	graphPadY  = 24
	dotRadius  = 4
)

// Layout holds the complete graph layout for SVG rendering.
type Layout struct {
	Groups []Group
	Edges  []Edge
	Width  int
	Height int
}

// Group is a column of nodes sharing the same dependency depth.
type Group struct {
	Nodes          []Node
	X, Y           int
	Width          int
	Height         int
	Column         int
	LeftX, LeftY   int // center-left connection point
	RightX, RightY int // center-right connection point
}

// Node is a single build step within a group.
type Node struct {
	ID       int64
	Name     string
	Status   string
	Duration string
	X, Y     int // absolute position within SVG
}

// Edge connects the right side of one group to the left side of another.
type Edge struct {
	FromX, FromY int
	ToX, ToY     int
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

	// Group steps by column
	maxCol := 0
	colSteps := make(map[int][]*models.BuildStep)
	for _, s := range steps {
		col := columns[s.Name]
		colSteps[col] = append(colSteps[col], s)
		if col > maxCol {
			maxCol = col
		}
	}

	// Build groups
	groups := make([]Group, 0, maxCol+1)
	groupByCol := make(map[int]*Group)

	x := graphPadX
	maxHeight := 0

	for col := 0; col <= maxCol; col++ {
		ss := colSteps[col]
		if len(ss) == 0 {
			continue
		}

		// Sort nodes within group by name for stable ordering
		sort.Slice(ss, func(i, j int) bool {
			return ss[i].Name < ss[j].Name
		})

		groupWidth := nodeWidth + 2*groupPadX
		groupHeight := 2*groupPadY + len(ss)*nodeHeight + (len(ss)-1)*nodeGapY

		g := Group{
			Column: col,
			X:      x,
			Width:  groupWidth,
			Height: groupHeight,
		}

		// Build nodes with positions
		for i, s := range ss {
			nodeY := groupPadY + i*(nodeHeight+nodeGapY)
			g.Nodes = append(g.Nodes, Node{
				ID:       s.ID,
				Name:     truncate(s.Name, 24),
				Status:   string(s.Status),
				Duration: formatStepDuration(s),
			})
			// Absolute Y will be set after vertical centering; store relative for now
			_ = nodeY
		}

		groups = append(groups, g)
		groupByCol[col] = &groups[len(groups)-1]

		if groupHeight > maxHeight {
			maxHeight = groupHeight
		}

		x += groupWidth + colGap
	}

	// Set vertical positions: center each group relative to tallest
	totalHeight := maxHeight + 2*graphPadY
	for i := range groups {
		groups[i].Y = graphPadY + (maxHeight-groups[i].Height)/2

		// Calculate absolute node positions
		for j := range groups[i].Nodes {
			groups[i].Nodes[j].X = groups[i].X + groupPadX
			groups[i].Nodes[j].Y = groups[i].Y + groupPadY + j*(nodeHeight+nodeGapY)
		}

		// Connection points
		groups[i].LeftX = groups[i].X
		groups[i].LeftY = groups[i].Y + groups[i].Height/2
		groups[i].RightX = groups[i].X + groups[i].Width
		groups[i].RightY = groups[i].Y + groups[i].Height/2
	}

	totalWidth := x - colGap + graphPadX

	// Build edges between groups based on dependencies
	// Map step name to its group column
	nameToCol := make(map[string]int, len(steps))
	for _, s := range steps {
		nameToCol[s.Name] = columns[s.Name]
	}

	// Track unique group-to-group edges (avoid duplicates)
	type edgeKey struct{ from, to int }
	seenEdges := make(map[edgeKey]bool)

	var edges []Edge
	for _, s := range steps {
		if len(s.DependsOn) == 0 {
			continue
		}
		toCol := columns[s.Name]
		toGroup := groupByCol[toCol]
		if toGroup == nil {
			continue
		}

		for _, depName := range s.DependsOn {
			fromCol, ok := nameToCol[depName]
			if !ok {
				continue
			}
			fromGroup := groupByCol[fromCol]
			if fromGroup == nil {
				continue
			}

			key := edgeKey{fromCol, toCol}
			if seenEdges[key] {
				continue
			}
			seenEdges[key] = true

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
