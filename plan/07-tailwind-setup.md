---
model: sonnet
---

# Step 07: Tailwind CSS Build System

## Objective
Set up Tailwind CSS compilation with the standalone CLI, embedded in the Go binary at build time.

## Tasks

### 7.1 Create Tailwind Configuration
Create `web/tailwind/tailwind.config.js`:
```javascript
/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "../templates/**/*.html",
  ],
  theme: {
    extend: {
      colors: {
        // Custom brand colors
        'feather': {
          50: '#f0f9ff',
          100: '#e0f2fe',
          500: '#0ea5e9',
          600: '#0284c7',
          700: '#0369a1',
        },
      },
    },
  },
  plugins: [],
}
```

### 7.2 Create Base CSS
Create `web/tailwind/input.css`:
```css
@tailwind base;
@tailwind components;
@tailwind utilities;

@layer components {
  .btn {
    @apply px-4 py-2 rounded-md font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2;
  }
  .btn-primary {
    @apply bg-feather-600 text-white hover:bg-feather-700 focus:ring-feather-500;
  }
  .btn-secondary {
    @apply bg-gray-100 text-gray-700 hover:bg-gray-200 focus:ring-gray-500;
  }
  .btn-danger {
    @apply bg-red-600 text-white hover:bg-red-700 focus:ring-red-500;
  }
  .input {
    @apply block w-full rounded-md border-gray-300 shadow-sm focus:border-feather-500 focus:ring-feather-500 sm:text-sm;
  }
  .card {
    @apply bg-white shadow rounded-lg;
  }
  .badge {
    @apply inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium;
  }
  .badge-success {
    @apply bg-green-100 text-green-800;
  }
  .badge-failure {
    @apply bg-red-100 text-red-800;
  }
  .badge-pending {
    @apply bg-yellow-100 text-yellow-800;
  }
  .badge-running {
    @apply bg-blue-100 text-blue-800;
  }
}
```

### 7.3 Update Makefile for CSS Build
```makefile
TAILWIND_VERSION := 3.4.1
TAILWIND_CLI := ./bin/tailwindcss

# Detect OS/Arch for Tailwind download
ifeq ($(shell uname -s),Darwin)
    ifeq ($(shell uname -m),arm64)
        TAILWIND_PLATFORM := macos-arm64
    else
        TAILWIND_PLATFORM := macos-x64
    endif
else
    ifeq ($(shell uname -m),aarch64)
        TAILWIND_PLATFORM := linux-arm64
    else
        TAILWIND_PLATFORM := linux-x64
    endif
endif

.PHONY: tailwind-download
tailwind-download:
	@mkdir -p bin
	@if [ ! -f $(TAILWIND_CLI) ]; then \
		curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/download/v$(TAILWIND_VERSION)/tailwindcss-$(TAILWIND_PLATFORM); \
		mv tailwindcss-$(TAILWIND_PLATFORM) $(TAILWIND_CLI); \
		chmod +x $(TAILWIND_CLI); \
	fi

.PHONY: css
css: tailwind-download
	$(TAILWIND_CLI) -c web/tailwind/tailwind.config.js -i web/tailwind/input.css -o web/static/css/main.css --minify

.PHONY: css-watch
css-watch: tailwind-download
	$(TAILWIND_CLI) -c web/tailwind/tailwind.config.js -i web/tailwind/input.css -o web/static/css/main.css --watch
```

### 7.4 Set Up Static File Embedding
Create `web/static/embed.go`:
```go
package static

import "embed"

//go:embed css/*.css js/*.js images/*
var Files embed.FS
```

### 7.5 Add HTMX
Download HTMX and place in `web/static/js/`:
```makefile
HTMX_VERSION := 1.9.10

.PHONY: htmx-download
htmx-download:
	@mkdir -p web/static/js
	@if [ ! -f web/static/js/htmx.min.js ]; then \
		curl -sL https://unpkg.com/htmx.org@$(HTMX_VERSION)/dist/htmx.min.js -o web/static/js/htmx.min.js; \
	fi
```

### 7.6 Update Build Target
```makefile
.PHONY: build
build: css htmx-download
	go build -o bin/featherci ./cmd/featherci

.PHONY: dev
dev: css htmx-download
	go run ./cmd/featherci
```

### 7.7 Add to .gitignore
```
# Don't ignore compiled CSS - it's needed for the build
# But do ignore Tailwind CLI binary
bin/tailwindcss
```

Actually, we should include the compiled CSS in git for reproducible builds:
```
# Include web/static/css/main.css in git
```

## Deliverables
- [ ] `web/tailwind/tailwind.config.js` - Tailwind configuration
- [ ] `web/tailwind/input.css` - Base styles
- [ ] `web/static/embed.go` - Static file embedding
- [ ] `web/static/css/main.css` - Compiled CSS
- [ ] `web/static/js/htmx.min.js` - HTMX library
- [ ] Updated `Makefile` with CSS targets
- [ ] CSS builds and is served correctly

## Dependencies
- Step 06: Web server (to serve static files)

## Estimated Effort
Small - Build tooling setup
