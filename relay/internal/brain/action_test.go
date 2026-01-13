package brain

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestGapSeverityUnmarshalNormalizesCase(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		severity string
		want     GapSeverity
	}{
		{name: "upper", severity: "MEDIUM", want: GapSeverityMedium},
		{name: "trimmed", severity: " Medium ", want: GapSeverityMedium},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(fmt.Sprintf(`{"actions":[{"type":"update_gaps","data":{"add":[{"question":"Q","severity":"%s","respondent":"assignee"}]}}]}`, tc.severity))

			var input SubmitActionsInput
			if err := json.Unmarshal(raw, &input); err != nil {
				t.Fatalf("unmarshal submit: %v", err)
			}

			if len(input.Actions) != 1 {
				t.Fatalf("expected 1 action, got %d", len(input.Actions))
			}

			data, err := ParseActionData[UpdateGapsAction](input.Actions[0])
			if err != nil {
				t.Fatalf("parse action data: %v", err)
			}

			if len(data.Add) != 1 {
				t.Fatalf("expected 1 gap, got %d", len(data.Add))
			}

			if got := data.Add[0].Severity; got != tc.want {
				t.Fatalf("severity = %q, want %q", got, tc.want)
			}
		})
	}
}
