---
model: sonnet
---

# Step 18: Log Streaming with SSE

## Objective
Implement Server-Sent Events (SSE) for real-time log streaming during builds.

## Tasks

### 18.1 Create Log Storage Interface
```go
type LogStorage interface {
    PathForStep(stepID int64) string
    Write(stepID int64, data []byte) error
    Append(stepID int64, data []byte) error
    Read(stepID int64, offset int64) ([]byte, error)
    Stream(ctx context.Context, stepID int64) (<-chan []byte, error)
    GetSize(stepID int64) (int64, error)
}
```

### 18.2 Implement File-based Log Storage
```go
type FileLogStorage struct {
    basePath string
}

func NewFileLogStorage(basePath string) *FileLogStorage {
    return &FileLogStorage{basePath: basePath}
}

func (s *FileLogStorage) PathForStep(stepID int64) string {
    return filepath.Join(s.basePath, fmt.Sprintf("step-%d.log", stepID))
}

func (s *FileLogStorage) Write(stepID int64, data []byte) error {
    path := s.PathForStep(stepID)
    return os.WriteFile(path, data, 0644)
}

func (s *FileLogStorage) Append(stepID int64, data []byte) error {
    path := s.PathForStep(stepID)
    f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return err
    }
    defer f.Close()
    _, err = f.Write(data)
    return err
}

func (s *FileLogStorage) Read(stepID int64, offset int64) ([]byte, error) {
    path := s.PathForStep(stepID)
    f, err := os.Open(path)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, nil
        }
        return nil, err
    }
    defer f.Close()
    
    if offset > 0 {
        f.Seek(offset, io.SeekStart)
    }
    
    return io.ReadAll(f)
}

func (s *FileLogStorage) Stream(ctx context.Context, stepID int64) (<-chan []byte, error) {
    ch := make(chan []byte, 10)
    
    go func() {
        defer close(ch)
        
        path := s.PathForStep(stepID)
        offset := int64(0)
        
        for {
            select {
            case <-ctx.Done():
                return
            default:
                data, _ := s.Read(stepID, offset)
                if len(data) > 0 {
                    ch <- data
                    offset += int64(len(data))
                }
                time.Sleep(500 * time.Millisecond)
            }
        }
    }()
    
    return ch, nil
}
```

### 18.3 Create SSE Handler
```go
type LogHandler struct {
    logs   LogStorage
    steps  BuildStepRepository
    builds BuildRepository
}

func (h *LogHandler) StreamLogs(w http.ResponseWriter, r *http.Request) {
    stepID := parseStepID(r)
    
    // Verify access
    step, err := h.steps.GetByID(r.Context(), stepID)
    if err != nil {
        http.Error(w, "step not found", http.StatusNotFound)
        return
    }
    
    // Set SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
    
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming not supported", http.StatusInternalServerError)
        return
    }
    
    ctx := r.Context()
    offset := int64(0)
    
    // Initial read of existing logs
    existingLogs, _ := h.logs.Read(stepID, 0)
    if len(existingLogs) > 0 {
        fmt.Fprintf(w, "data: %s\n\n", escapeSSE(existingLogs))
        flusher.Flush()
        offset = int64(len(existingLogs))
    }
    
    // Stream new logs
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // Check if step is still running
            step, _ = h.steps.GetByID(ctx, stepID)
            
            newLogs, _ := h.logs.Read(stepID, offset)
            if len(newLogs) > 0 {
                fmt.Fprintf(w, "data: %s\n\n", escapeSSE(newLogs))
                flusher.Flush()
                offset += int64(len(newLogs))
            }
            
            // Stop streaming if step is complete
            if step.Status != StepStatusRunning {
                fmt.Fprintf(w, "event: complete\ndata: %s\n\n", step.Status)
                flusher.Flush()
                return
            }
        }
    }
}

func escapeSSE(data []byte) string {
    // Replace newlines with SSE-compatible format
    // Each line needs "data: " prefix
    lines := bytes.Split(data, []byte("\n"))
    var result strings.Builder
    for i, line := range lines {
        if i > 0 {
            result.WriteString("\ndata: ")
        }
        result.Write(line)
    }
    return result.String()
}
```

### 18.4 Create HTMX-compatible Log Endpoint
For non-SSE clients, provide a simple polling endpoint:
```go
func (h *LogHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
    stepID := parseStepID(r)
    
    logs, err := h.logs.Read(stepID, 0)
    if err != nil {
        http.Error(w, "error reading logs", http.StatusInternalServerError)
        return
    }
    
    w.Header().Set("Content-Type", "text/html")
    fmt.Fprintf(w, `<pre class="text-sm text-gray-100 font-mono whitespace-pre-wrap"><code>%s</code></pre>`,
        html.EscapeString(string(logs)))
}
```

### 18.5 Create Log Viewer JavaScript
`web/static/js/log-viewer.js`:
```javascript
class LogViewer {
    constructor(element, url) {
        this.element = element;
        this.url = url;
        this.eventSource = null;
        this.init();
    }
    
    init() {
        this.eventSource = new EventSource(this.url);
        
        this.eventSource.onmessage = (event) => {
            this.appendLog(event.data);
        };
        
        this.eventSource.addEventListener('complete', (event) => {
            this.eventSource.close();
            this.element.classList.add('log-complete');
        });
        
        this.eventSource.onerror = () => {
            this.eventSource.close();
        };
    }
    
    appendLog(data) {
        const code = this.element.querySelector('code');
        code.textContent += data;
        // Auto-scroll to bottom
        this.element.scrollTop = this.element.scrollHeight;
    }
    
    destroy() {
        if (this.eventSource) {
            this.eventSource.close();
        }
    }
}

// Auto-initialize log viewers
document.addEventListener('DOMContentLoaded', () => {
    document.querySelectorAll('[data-log-stream]').forEach(el => {
        new LogViewer(el, el.dataset.logStream);
    });
});
```

### 18.6 Add ANSI Color Support
```go
func ansiToHTML(input string) string {
    // Convert ANSI escape codes to HTML spans
    // Common codes:
    // \x1b[0m - reset
    // \x1b[1m - bold
    // \x1b[31m - red
    // \x1b[32m - green
    // etc.
    
    // Use a library or simple regex replacement
}
```

### 18.7 Add Tests
- Test log writing/reading
- Test SSE streaming
- Test ANSI conversion
- Test concurrent access

## Deliverables
- [ ] `internal/logs/storage.go` - Log storage implementation
- [ ] `internal/handlers/logs.go` - Log HTTP handlers
- [ ] `web/static/js/log-viewer.js` - Log viewer JS
- [ ] SSE streaming works in browser
- [ ] Logs persist and are readable after build completes

## Dependencies
- Step 14: Docker executor (writes logs)
- Step 17: Build status UI

## Estimated Effort
Medium - Real-time streaming implementation
