package brain

import (
	"testing"
)

func TestSanitizeComment(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantContent  string
		wantStripped int
	}{
		{
			name:         "gap ID at start of line",
			input:        "[gap 17] What is the expected behavior?",
			wantContent:  "What is the expected behavior?",
			wantStripped: 1,
		},
		{
			name:         "multiple gap IDs",
			input:        "1. [gap 17] First question\n2. [gap 18] Second question",
			wantContent:  "1. First question\n2. Second question",
			wantStripped: 2,
		},
		{
			name:         "no gap IDs",
			input:        "Just a normal comment with no internal markers.",
			wantContent:  "Just a normal comment with no internal markers.",
			wantStripped: 0,
		},
		{
			name:         "gap ID mid-sentence",
			input:        "Regarding [gap 17], we should clarify this.",
			wantContent:  "Regarding , we should clarify this.",
			wantStripped: 1,
		},
		{
			name:         "gap ID with extra whitespace",
			input:        "[gap  42] Question with extra space in marker",
			wantContent:  "Question with extra space in marker",
			wantStripped: 1,
		},
		{
			name:         "gap ID without trailing space",
			input:        "[gap 17]Question directly after",
			wantContent:  "Question directly after",
			wantStripped: 1,
		},
		{
			name:         "large gap ID",
			input:        "[gap 12345] Question with large ID",
			wantContent:  "Question with large ID",
			wantStripped: 1,
		},
		{
			name:         "empty content",
			input:        "",
			wantContent:  "",
			wantStripped: 0,
		},
		{
			name:         "only gap ID",
			input:        "[gap 1] ",
			wantContent:  "",
			wantStripped: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotContent, gotStripped := SanitizeComment(tt.input)
			if gotContent != tt.wantContent {
				t.Errorf("SanitizeComment() content = %q, want %q", gotContent, tt.wantContent)
			}
			if gotStripped != tt.wantStripped {
				t.Errorf("SanitizeComment() stripped = %d, want %d", gotStripped, tt.wantStripped)
			}
		})
	}
}
