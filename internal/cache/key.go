package cache

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

var checksumRegex = regexp.MustCompile(`\{\{\s*checksum\s+"([^"]+)"\s*\}\}`)

// ResolveKey resolves a cache key template into a concrete key.
// It substitutes {{ .Branch }} with the branch name and
// {{ checksum "filename" }} with the first 16 hex chars of the file's SHA-256.
func ResolveKey(tmpl string, projectID int64, branch string, workspacePath string) string {
	// First, handle checksum functions (not supported by text/template natively)
	resolved := checksumRegex.ReplaceAllStringFunc(tmpl, func(match string) string {
		sub := checksumRegex.FindStringSubmatch(match)
		if len(sub) < 2 {
			return "unknown"
		}
		filename := sub[1]
		return fileChecksum(filepath.Join(workspacePath, filename))
	})

	// Then handle Go template variables
	t, err := template.New("key").Parse(resolved)
	if err != nil {
		return fmt.Sprintf("project-%d-%s", projectID, resolved)
	}

	data := struct{ Branch string }{Branch: branch}
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return fmt.Sprintf("project-%d-%s", projectID, resolved)
	}

	return fmt.Sprintf("project-%d-%s", projectID, buf.String())
}

func fileChecksum(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "unknown"
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:8])
}
