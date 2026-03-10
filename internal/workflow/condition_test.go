package workflow

import (
	"testing"
)

func TestEvaluateCondition(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		vars     map[string]string
		expected bool
		wantErr  bool
	}{
		{
			name:     "empty expression",
			expr:     "",
			vars:     map[string]string{"branch": "main"},
			expected: true,
		},
		{
			name:     "exact match true",
			expr:     `branch == "main"`,
			vars:     map[string]string{"branch": "main"},
			expected: true,
		},
		{
			name:     "exact match false",
			expr:     `branch == "main"`,
			vars:     map[string]string{"branch": "develop"},
			expected: false,
		},
		{
			name:     "not equal true",
			expr:     `branch != "dev"`,
			vars:     map[string]string{"branch": "main"},
			expected: true,
		},
		{
			name:     "not equal false",
			expr:     `branch != "main"`,
			vars:     map[string]string{"branch": "main"},
			expected: false,
		},
		{
			name:     "glob match true",
			expr:     `branch =~ "release/*"`,
			vars:     map[string]string{"branch": "release/1.0"},
			expected: true,
		},
		{
			name:     "glob match false",
			expr:     `branch =~ "release/*"`,
			vars:     map[string]string{"branch": "feature/foo"},
			expected: false,
		},
		{
			name:     "negative glob true",
			expr:     `branch !~ "feature/*"`,
			vars:     map[string]string{"branch": "main"},
			expected: true,
		},
		{
			name:     "negative glob false",
			expr:     `branch !~ "feature/*"`,
			vars:     map[string]string{"branch": "feature/foo"},
			expected: false,
		},
		{
			name:     "unquoted value",
			expr:     `branch == main`,
			vars:     map[string]string{"branch": "main"},
			expected: true,
		},
		{
			name:    "unknown variable",
			expr:    `tag == "v1.0"`,
			vars:    map[string]string{"branch": "main"},
			wantErr: true,
		},
		{
			name:    "missing operator",
			expr:    `branch main`,
			wantErr: true,
		},
		{
			name:     "wildcard glob",
			expr:     `branch =~ "*"`,
			vars:     map[string]string{"branch": "anything"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EvaluateCondition(tt.expr, tt.vars)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestValidateCondition(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"empty", "", false},
		{"valid equals", `branch == "main"`, false},
		{"valid not equals", `branch != "dev"`, false},
		{"valid glob", `branch =~ "release/*"`, false},
		{"valid negative glob", `branch !~ "feature/*"`, false},
		{"unknown variable", `tag == "v1"`, true},
		{"missing operator", `branch main`, true},
		{"missing value", `branch == ""`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCondition(tt.expr)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
