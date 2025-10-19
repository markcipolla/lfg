package github

import (
	"testing"
)

func TestEscapeString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "escape backslash",
			input:    `test\test`,
			expected: `test\\test`,
		},
		{
			name:     "escape quotes",
			input:    `test"test`,
			expected: `test\"test`,
		},
		{
			name:     "escape newline",
			input:    "test\ntest",
			expected: `test\ntest`,
		},
		{
			name:     "escape backslash and quote",
			input:    `test\"test`,
			expected: `test\\\"test`,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeString(tt.input)
			if result != tt.expected {
				t.Errorf("escapeString(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
