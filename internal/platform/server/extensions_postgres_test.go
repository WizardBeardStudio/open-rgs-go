package server

import (
	"testing"

	rgsv1 "github.com/wizardbeardstudio/open-rgs-go/gen/rgs/v1"
)

func TestParsePromotionalAwardType(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    rgsv1.PromotionalAwardType
		wantErr bool
	}{
		{name: "valid", raw: "1", want: rgsv1.PromotionalAwardType_PROMOTIONAL_AWARD_TYPE_FREEPLAY},
		{name: "non-numeric", raw: "abc", wantErr: true},
		{name: "unsupported", raw: "99", wantErr: true},
		{name: "unspecified", raw: "0", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePromotionalAwardType(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected type: got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestParseSystemWindowEventType(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    rgsv1.SystemWindowEventType
		wantErr bool
	}{
		{name: "valid", raw: "1", want: rgsv1.SystemWindowEventType_SYSTEM_WINDOW_EVENT_TYPE_OPENED},
		{name: "non-numeric", raw: "abc", wantErr: true},
		{name: "unsupported", raw: "99", wantErr: true},
		{name: "unspecified", raw: "0", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSystemWindowEventType(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected type: got=%v want=%v", got, tc.want)
			}
		})
	}
}
