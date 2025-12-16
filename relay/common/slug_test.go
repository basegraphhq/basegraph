package common

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fallback string
		want     string
		wantErr  bool
	}{
		{"simple", "Hello World", "default", "hello-world", false},
		{"with special chars", "Hello@World!", "default", "hello-world", false},
		{"preserves numbers", "Test 123", "default", "test-123", false},
		{"trims hyphens", "---test---", "default", "test", false},
		{"uses fallback when empty", "", "fallback", "fallback", false},
		{"uses fallback when whitespace only", "   ", "fallback", "fallback", false},
		{"uses fallback when special chars only", "@#$%", "fallback", "fallback", false},
		{"error when both empty", "", "", "", true},
		{"error when both result in empty", "@#$", "!@#", "", true},
		{"already lowercase", "hello-world", "default", "hello-world", false},
		{"mixed case", "HeLLo WoRLD", "default", "hello-world", false},
		{"multiple spaces", "hello    world", "default", "hello-world", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Slugify(tt.input, tt.fallback)
			if (err != nil) != tt.wantErr {
				t.Errorf("Slugify() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Slugify() = %q, want %q", got, tt.want)
			}
		})
	}
}
