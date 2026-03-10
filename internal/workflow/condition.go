package workflow

import (
	"fmt"
	"strings"
)

// EvaluateCondition evaluates a condition expression against the given variables.
// Returns true if the condition is met (or if the expression is empty).
// Supported syntax: variable operator "value"
// Variables: branch
// Operators: == (equal), != (not equal), =~ (glob match), !~ (negative glob match)
func EvaluateCondition(expr string, vars map[string]string) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}

	variable, operator, value, err := parseCondition(expr)
	if err != nil {
		return false, err
	}

	actual, ok := vars[variable]
	if !ok {
		return false, fmt.Errorf("unknown variable %q in condition", variable)
	}

	switch operator {
	case "==":
		return actual == value, nil
	case "!=":
		return actual != value, nil
	case "=~":
		return MatchGlob(value, actual), nil
	case "!~":
		return !MatchGlob(value, actual), nil
	default:
		return false, fmt.Errorf("unknown operator %q in condition", operator)
	}
}

// ValidateCondition checks that a condition expression is syntactically valid.
// Returns nil if the expression is valid (or empty).
func ValidateCondition(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}

	variable, operator, _, err := parseCondition(expr)
	if err != nil {
		return err
	}

	// Validate variable name
	validVars := map[string]bool{"branch": true}
	if !validVars[variable] {
		return fmt.Errorf("unknown variable %q in condition; supported: branch", variable)
	}

	// Validate operator
	validOps := map[string]bool{"==": true, "!=": true, "=~": true, "!~": true}
	if !validOps[operator] {
		return fmt.Errorf("unknown operator %q in condition; supported: ==, !=, =~, !~", operator)
	}

	return nil
}

// parseCondition parses "variable operator \"value\"" into its three components.
func parseCondition(expr string) (variable, operator, value string, err error) {
	// Find operator (try two-char operators first)
	operators := []string{"==", "!=", "=~", "!~"}
	opIdx := -1
	opLen := 0
	for _, op := range operators {
		idx := strings.Index(expr, op)
		if idx != -1 && (opIdx == -1 || idx < opIdx) {
			opIdx = idx
			opLen = len(op)
			operator = op
		}
	}

	if opIdx == -1 {
		return "", "", "", fmt.Errorf("invalid condition %q: no operator found (supported: ==, !=, =~, !~)", expr)
	}

	variable = strings.TrimSpace(expr[:opIdx])
	rawValue := strings.TrimSpace(expr[opIdx+opLen:])

	if variable == "" {
		return "", "", "", fmt.Errorf("invalid condition %q: missing variable", expr)
	}

	// Strip quotes from value
	if len(rawValue) >= 2 && rawValue[0] == '"' && rawValue[len(rawValue)-1] == '"' {
		value = rawValue[1 : len(rawValue)-1]
	} else {
		value = rawValue
	}

	if value == "" {
		return "", "", "", fmt.Errorf("invalid condition %q: missing value", expr)
	}

	return variable, operator, value, nil
}
