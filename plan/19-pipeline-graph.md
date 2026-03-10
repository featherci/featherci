---
model: opus
---

# Step 19: Pipeline Graph Visualization

## Objective
Implement SVG-based pipeline graph visualization showing step dependencies and status.

## Tasks

### 19.1 Create Graph Layout Algorithm
```go
type GraphLayout struct {
    Nodes  []Node
    Edges  []Edge
    Width  int
    Height int
}

type Node struct {
    ID       int64
    Name     string
    Status   StepStatus
    Duration string
    X        int
    Y        int
    Width    int
    Height   int
    Column   int
    Row      int
}

type Edge struct {
    FromID int64
    ToID   int64
    Points []Point
}

type Point struct {
    X int
    Y int
}

func CalculateLayout(steps []*BuildStep) *GraphLayout {
    // 1. Build adjacency list
    deps := make(map[int64][]int64)
    rdeps := make(map[int64][]int64) // reverse dependencies
    
    for _, step := range steps {
        for _, depID := range step.DependencyIDs {
            deps[step.ID] = append(deps[step.ID], depID)
            rdeps[depID] = append(rdeps[depID], step.ID)
        }
    }
    
    // 2. Topological sort to determine columns
    columns := assignColumns(steps, deps)
    
    // 3. Within each column, assign rows
    rows := assignRows(steps, columns, rdeps)
    
    // 4. Calculate positions
    const (
        nodeWidth  = 200
        nodeHeight = 60
        colGap     = 80
        rowGap     = 20
        padding    = 40
    )
    
    layout := &GraphLayout{}
    
    for _, step := range steps {
        node := Node{
            ID:       step.ID,
            Name:     step.Name,
            Status:   step.Status,
            Duration: formatDuration(step.StartedAt, step.FinishedAt),
            Column:   columns[step.ID],
            Row:      rows[step.ID],
            Width:    nodeWidth,
            Height:   nodeHeight,
            X:        padding + columns[step.ID]*(nodeWidth+colGap),
            Y:        padding + rows[step.ID]*(nodeHeight+rowGap),
        }
        layout.Nodes = append(layout.Nodes, node)
    }
    
    // 5. Calculate edges with bezier curves
    nodeMap := make(map[int64]Node)
    for _, n := range layout.Nodes {
        nodeMap[n.ID] = n
    }
    
    for _, step := range steps {
        for _, depID := range step.DependencyIDs {
            from := nodeMap[depID]
            to := nodeMap[step.ID]
            
            edge := Edge{
                FromID: depID,
                ToID:   step.ID,
                Points: calculateBezierPoints(from, to),
            }
            layout.Edges = append(layout.Edges, edge)
        }
    }
    
    // 6. Calculate total dimensions
    layout.Width = findMaxX(layout.Nodes) + nodeWidth + padding
    layout.Height = findMaxY(layout.Nodes) + nodeHeight + padding
    
    return layout
}
```

### 19.2 Implement Column Assignment (Topological Sort)
```go
func assignColumns(steps []*BuildStep, deps map[int64][]int64) map[int64]int {
    columns := make(map[int64]int)
    inDegree := make(map[int64]int)
    
    for _, step := range steps {
        inDegree[step.ID] = len(deps[step.ID])
    }
    
    // Find steps with no dependencies (column 0)
    queue := []int64{}
    for _, step := range steps {
        if inDegree[step.ID] == 0 {
            queue = append(queue, step.ID)
            columns[step.ID] = 0
        }
    }
    
    // BFS to assign columns
    for len(queue) > 0 {
        current := queue[0]
        queue = queue[1:]
        
        // Find dependents
        for _, step := range steps {
            for _, depID := range deps[step.ID] {
                if depID == current {
                    // Update column to be max of all dependencies + 1
                    newCol := columns[current] + 1
                    if newCol > columns[step.ID] {
                        columns[step.ID] = newCol
                    }
                    
                    inDegree[step.ID]--
                    if inDegree[step.ID] == 0 {
                        queue = append(queue, step.ID)
                    }
                }
            }
        }
    }
    
    return columns
}
```

### 19.3 Create SVG Renderer
```go
func RenderSVG(layout *GraphLayout) string {
    var buf bytes.Buffer
    
    fmt.Fprintf(&buf, `<svg width="%d" height="%d" xmlns="http://www.w3.org/2000/svg">`, 
        layout.Width, layout.Height)
    
    // Define arrow marker
    buf.WriteString(`
        <defs>
            <marker id="arrowhead" markerWidth="10" markerHeight="7" 
                    refX="9" refY="3.5" orient="auto">
                <polygon points="0 0, 10 3.5, 0 7" fill="#9CA3AF"/>
            </marker>
        </defs>
    `)
    
    // Draw edges first (behind nodes)
    for _, edge := range layout.Edges {
        buf.WriteString(renderEdge(edge))
    }
    
    // Draw nodes
    for _, node := range layout.Nodes {
        buf.WriteString(renderNode(node))
    }
    
    buf.WriteString(`</svg>`)
    
    return buf.String()
}

func renderNode(n Node) string {
    bgColor := statusToBackground(n.Status)
    borderColor := statusToBorder(n.Status)
    iconSVG := statusToIconSVG(n.Status)
    
    return fmt.Sprintf(`
        <g transform="translate(%d, %d)">
            <rect width="%d" height="%d" rx="6" ry="6" 
                  fill="%s" stroke="%s" stroke-width="1"/>
            <g transform="translate(12, 20)">%s</g>
            <text x="40" y="25" font-family="system-ui" font-size="14" font-weight="500" fill="#111827">
                %s
            </text>
            <text x="%d" y="25" font-family="system-ui" font-size="12" fill="#6B7280" text-anchor="end">
                %s
            </text>
        </g>
    `, n.X, n.Y, n.Width, n.Height, bgColor, borderColor, iconSVG, n.Name, n.Width-12, n.Duration)
}

func renderEdge(e Edge) string {
    if len(e.Points) < 2 {
        return ""
    }
    
    // Create bezier curve path
    var path strings.Builder
    path.WriteString(fmt.Sprintf("M %d %d", e.Points[0].X, e.Points[0].Y))
    
    if len(e.Points) == 4 {
        // Cubic bezier
        path.WriteString(fmt.Sprintf(" C %d %d, %d %d, %d %d",
            e.Points[1].X, e.Points[1].Y,
            e.Points[2].X, e.Points[2].Y,
            e.Points[3].X, e.Points[3].Y))
    } else {
        // Simple line
        for i := 1; i < len(e.Points); i++ {
            path.WriteString(fmt.Sprintf(" L %d %d", e.Points[i].X, e.Points[i].Y))
        }
    }
    
    return fmt.Sprintf(`<path d="%s" fill="none" stroke="#9CA3AF" stroke-width="2" marker-end="url(#arrowhead)"/>`,
        path.String())
}
```

### 19.4 Create Status Colors
```go
func statusToBackground(status StepStatus) string {
    switch status {
    case StepStatusSuccess:
        return "#F0FDF4" // green-50
    case StepStatusFailure:
        return "#FEF2F2" // red-50
    case StepStatusRunning:
        return "#EFF6FF" // blue-50
    case StepStatusWaitingApproval:
        return "#FFFBEB" // yellow-50
    default:
        return "#FFFFFF" // white
    }
}

func statusToBorder(status StepStatus) string {
    switch status {
    case StepStatusSuccess:
        return "#22C55E" // green-500
    case StepStatusFailure:
        return "#EF4444" // red-500
    case StepStatusRunning:
        return "#3B82F6" // blue-500
    case StepStatusWaitingApproval:
        return "#F59E0B" // yellow-500
    default:
        return "#E5E7EB" // gray-200
    }
}
```

### 19.5 Create HTTP Handler
```go
func (h *BuildHandler) Graph(w http.ResponseWriter, r *http.Request) {
    buildNumber := parseBuildNumber(r)
    projectID := getProjectID(r)
    
    build, _ := h.builds.GetByNumber(r.Context(), projectID, buildNumber)
    steps, _ := h.steps.ListByBuild(r.Context(), build.ID)
    
    layout := CalculateLayout(steps)
    svg := RenderSVG(layout)
    
    w.Header().Set("Content-Type", "image/svg+xml")
    w.Write([]byte(svg))
}
```

### 19.6 Handle Grouped Steps
For steps that share the same dependencies (like the image shows multiple steps in a column), group them visually:
```go
type NodeGroup struct {
    Nodes  []Node
    X      int
    Y      int
    Width  int
    Height int
}

func groupNodes(nodes []Node) []NodeGroup {
    // Group nodes by column
    // If multiple nodes in same column share same dependencies, group them
}
```

### 19.7 Add Tests
- Test column assignment
- Test row assignment
- Test SVG rendering
- Test edge path calculation

## Deliverables
- [ ] `internal/graph/layout.go` - Graph layout algorithm
- [ ] `internal/graph/render.go` - SVG rendering
- [ ] `internal/handlers/graph.go` - HTTP handler
- [ ] Graph displays correctly in UI
- [ ] Graph updates with build status

## Dependencies
- Step 12: Build model
- Step 17: Build status UI

## Estimated Effort
Large - Complex visualization logic
