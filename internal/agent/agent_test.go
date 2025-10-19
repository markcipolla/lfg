package agent

import (
	"encoding/json"
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

func TestProcessLogEntry(t *testing.T) {
	// Skip this test as it requires mocking the GitHub API
	// The parsing logic is tested in TestJSONLEntryParsing
	t.Skip("Skipping test that requires mocking GitHub API")
}

func TestFindLatestSession(t *testing.T) {
	// Since findLatestSession uses UserHomeDir and Getwd, we can't easily test it
	// without mocking those functions. This test serves as documentation of the expected behavior.
	t.Skip("Skipping test that requires mocking system calls")
}

func TestJSONLEntryParsing(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		expectError bool
		expectedType string
		expectedText string
	}{
		{
			name:        "user message",
			json:        `{"type":"user","text":"test message"}`,
			expectError: false,
			expectedType: "user",
			expectedText: "test message",
		},
		{
			name:        "assistant message",
			json:        `{"type":"assistant","text":"response message"}`,
			expectError: false,
			expectedType: "assistant",
			expectedText: "response message",
		},
		{
			name:        "message with content field",
			json:        `{"type":"user","content":{"text":"nested text"}}`,
			expectError: false,
			expectedType: "user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entry JSONLEntry
			err := json.Unmarshal([]byte(tt.json), &entry)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error parsing JSON, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if entry.Type != tt.expectedType {
					t.Errorf("got type %q, want %q", entry.Type, tt.expectedType)
				}
				if tt.expectedText != "" && entry.Text != tt.expectedText {
					t.Errorf("got text %q, want %q", entry.Text, tt.expectedText)
				}
			}
		})
	}
}
