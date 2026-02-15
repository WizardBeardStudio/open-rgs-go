package evidence

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSummaryJSONFixtures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		file    string
		wantErr bool
	}{
		{name: "valid_v2_pass", file: "valid_v2_pass.json", wantErr: false},
		{name: "invalid_schema_version", file: "invalid_schema_version.json", wantErr: true},
		{name: "invalid_required_counts", file: "invalid_required_counts.json", wantErr: true},
		{name: "invalid_failed_step", file: "invalid_failed_step.json", wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join("testdata", tc.file)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture %s: %v", path, err)
			}

			err = ValidateSummaryJSON(data)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for fixture %s", path)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for fixture %s: %v", path, err)
			}
		})
	}
}
