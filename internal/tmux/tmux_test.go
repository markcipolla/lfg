package tmux

import (
	"testing"
)

func TestSanitizeSessionName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "dots replaced with underscores",
			input:    "household.email-homepage",
			expected: "household_email-homepage",
		},
		{
			name:     "multiple dots",
			input:    "my.project.name-feature",
			expected: "my_project_name-feature",
		},
		{
			name:     "no dots",
			input:    "project-feature",
			expected: "project-feature",
		},
		{
			name:     "only dots",
			input:    "...",
			expected: "___",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeSessionName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeSessionName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsInstalled(t *testing.T) {
	// This test checks if tmux command is available
	result := IsInstalled()
	// We don't assert true/false as it depends on system
	t.Logf("tmux installed: %v", result)
}
