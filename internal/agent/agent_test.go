package agent

import (
	"testing"
)

func TestExtractIssueNumber(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expected    int
		expectError bool
	}{
		{
			name:        "valid GitHub issue URL",
			url:         "https://github.com/owner/repo/issues/123",
			expected:    123,
			expectError: false,
		},
		{
			name:        "valid GitHub issue URL with trailing slash",
			url:         "https://github.com/owner/repo/issues/456/",
			expected:    456,
			expectError: false,
		},
		{
			name:        "empty URL",
			url:         "",
			expected:    0,
			expectError: true,
		},
		{
			name:        "invalid URL format",
			url:         "not-a-url",
			expected:    0,
			expectError: true,
		},
		{
			name:        "URL without issue number",
			url:         "https://github.com/owner/repo/issues/",
			expected:    0,
			expectError: true,
		},
		{
			name:        "URL with non-numeric issue",
			url:         "https://github.com/owner/repo/issues/abc",
			expected:    0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractIssueNumber(tt.url)

			if tt.expectError {
				if err == nil {
					t.Errorf("extractIssueNumber(%q) expected error, got nil", tt.url)
				}
			} else {
				if err != nil {
					t.Errorf("extractIssueNumber(%q) unexpected error: %v", tt.url, err)
				}
				if result != tt.expected {
					t.Errorf("extractIssueNumber(%q) = %d, want %d", tt.url, result, tt.expected)
				}
			}
		})
	}
}
