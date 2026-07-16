package compose

import "testing"

func TestParsePullProgress(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		want     PullProgressMsg
		wantJSON bool
	}{
		{
			name:     "downloading layer uses parent image and byte percentage",
			line:     `{"id":"7a2c55901189","parent_id":"Image registry.example/alice/backend:latest","status":"Working","text":"Downloading","details":"25B","current":25,"total":100,"percent":25}`,
			want:     PullProgressMsg{Service: "registry.example/alice/backend:latest", Status: "Downloading", Percent: 25, HasPercent: true, Raw: "registry.example/alice/backend:latest Downloading 25%"},
			wantJSON: true,
		},
		{
			name:     "extracting computes percentage from bytes",
			line:     `{"id":"7a2c55901189","parent_id":"Image alice/backend:latest","status":"Working","text":"Extracting","current":3,"total":4}`,
			want:     PullProgressMsg{Service: "alice/backend:latest", Status: "Extracting", Percent: 75, HasPercent: true, Raw: "alice/backend:latest Extracting 75%"},
			wantJSON: true,
		},
		{
			name:     "plain downloading layer reports byte percentage",
			line:     "7a2c55901189 Downloading 2.5MB / 10MB",
			want:     PullProgressMsg{Service: "layer 7a2c55901189", Status: "Downloading", Percent: 25, HasPercent: true, Raw: "layer 7a2c55901189 Downloading 25%"},
			wantJSON: false,
		},
		{
			name:     "complete image has no invented percentage",
			line:     `{"id":"Image alice/backend:latest","status":"Done","text":"Pulled"}`,
			want:     PullProgressMsg{Service: "alice/backend:latest", Status: "Pulled", Raw: "alice/backend:latest Pulled"},
			wantJSON: true,
		},
		{
			name:     "unknown structured event remains visible",
			line:     `{"id":"Image alice/backend:latest","status":"Working","text":"Waiting"}`,
			want:     PullProgressMsg{Service: "alice/backend:latest", Status: "Waiting", Raw: "alice/backend:latest Waiting"},
			wantJSON: true,
		},
		{
			name:     "image digest is omitted from label",
			line:     `{"id":"Image registry.example/alice/backend@sha256:1234567890abcdef","status":"Working","text":"Pulling"}`,
			want:     PullProgressMsg{Service: "registry.example/alice/backend", Status: "Pulling", Raw: "registry.example/alice/backend Pulling"},
			wantJSON: true,
		},
		{
			name:     "plain line is preserved",
			line:     "registry timeout while pulling image",
			want:     PullProgressMsg{Status: "Pulling", Raw: "registry timeout while pulling image"},
			wantJSON: false,
		},
		{
			name:     "unrelated JSON diagnostic is preserved",
			line:     `{"level":"error","msg":"registry unavailable"}`,
			want:     PullProgressMsg{Status: "Error", Raw: `{"level":"error","msg":"registry unavailable"}`},
			wantJSON: false,
		},
		{
			name:     "error details remain actionable",
			line:     `{"id":"Image registry.example/alice/backend:latest","status":"Error","text":"Error","details":"pull access denied"}`,
			want:     PullProgressMsg{Service: "registry.example/alice/backend:latest", Status: "Error: pull access denied", Raw: "registry.example/alice/backend:latest Error: pull access denied"},
			wantJSON: true,
		},
		{
			name:     "error text remains actionable without details",
			line:     `{"id":"Image registry.example/alice/backend:latest","status":"Error","text":"manifest unknown"}`,
			want:     PullProgressMsg{Service: "registry.example/alice/backend:latest", Status: "Error: manifest unknown", Raw: "registry.example/alice/backend:latest Error: manifest unknown"},
			wantJSON: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotJSON := parsePullProgress(tt.line)
			if got != tt.want || gotJSON != tt.wantJSON {
				t.Fatalf("parsePullProgress() = (%+v, %v), want (%+v, %v)", got, gotJSON, tt.want, tt.wantJSON)
			}
		})
	}
}
