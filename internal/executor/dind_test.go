package executor

import (
	"testing"
)

func TestPathMapper_Map(t *testing.T) {
	pm := &pathMapper{
		mappings: []mountMapping{
			{containerPath: "/data/workspaces", hostPath: "/var/lib/docker/volumes/featherci/_data/workspaces"},
			{containerPath: "/data", hostPath: "/var/lib/docker/volumes/featherci/_data"},
		},
	}

	tests := []struct {
		input string
		want  string
	}{
		{"/data/workspaces/4/15", "/var/lib/docker/volumes/featherci/_data/workspaces/4/15"},
		{"/data/cache", "/var/lib/docker/volumes/featherci/_data/cache"},
		{"/data", "/var/lib/docker/volumes/featherci/_data"},
		{"/other/path", "/other/path"}, // no match
		{"/datafoo", "/datafoo"},       // no false prefix match
	}

	for _, tt := range tests {
		got := pm.Map(tt.input)
		if got != tt.want {
			t.Errorf("Map(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPathMapper_Nil(t *testing.T) {
	var pm *pathMapper
	got := pm.Map("/data/workspaces/4/15")
	if got != "/data/workspaces/4/15" {
		t.Errorf("nil mapper should return path unchanged, got %q", got)
	}
}
