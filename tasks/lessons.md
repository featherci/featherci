# Lessons Learned

## Go Templates with Multiple Page Templates

**Problem**: When using Go's `html/template` with multiple page templates that define the same blocks (e.g., `{{define "title"}}` and `{{define "content"}}`), later template definitions overwrite earlier ones because all templates are parsed into a single namespace.

**Symptom**: All pages render the content from whichever page template was parsed last (alphabetically by filename).

**Solution**: Clone the base template for each page template, so each page has its own isolated namespace for block definitions:

```go
// First pass: load layouts and components into base template
base := template.New("").Funcs(funcs)
// ... parse layouts/base.html, components/*.html

// Second pass: clone base for each page
pages := make(map[string]*template.Template)
for each page in pages/*.html {
    pageTemplate, _ := base.Clone()
    pageTemplate.New(pagePath).Parse(pageContent)
    pages[pagePath] = pageTemplate
}
```

**Files affected**: `internal/templates/engine.go`

---

## Tailwind CSS Content Paths with Standalone CLI

**Problem**: When using Tailwind's standalone CLI with a config file, content paths are NOT resolved relative to the config file location. They're resolved relative to the current working directory when running the CLI.

**Symptom**: CSS compiles without errors but utility classes from templates are missing. Page renders as unstyled content with oversized SVGs.

**Wrong** (paths relative to config file):
```js
// web/tailwind/tailwind.config.js
content: [
  "../templates/**/*.html",  // WRONG - relative to config file
]
```

**Correct** (paths relative to project root where CLI is invoked):
```js
// web/tailwind/tailwind.config.js  
content: [
  "./web/templates/**/*.html",  // CORRECT - relative to project root
]
```

**Files affected**: `web/tailwind/tailwind.config.js`
