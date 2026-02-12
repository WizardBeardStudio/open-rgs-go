package main

import "testing"

func TestValidateProductionRuntimeStrictRequirements(t *testing.T) {
	cases := []struct {
		name          string
		strict        bool
		databaseURL   string
		tlsEnabled    bool
		jwtSecret     string
		jwtKeysetSpec string
		wantErr       bool
	}{
		{
			name:          "non-strict allows dev defaults",
			strict:        false,
			databaseURL:   "",
			tlsEnabled:    false,
			jwtSecret:     "dev-insecure-change-me",
			jwtKeysetSpec: "",
			wantErr:       false,
		},
		{
			name:          "strict requires database",
			strict:        true,
			databaseURL:   "",
			tlsEnabled:    true,
			jwtSecret:     "prod-secret",
			jwtKeysetSpec: "",
			wantErr:       true,
		},
		{
			name:          "strict requires tls",
			strict:        true,
			databaseURL:   "postgres://x",
			tlsEnabled:    false,
			jwtSecret:     "prod-secret",
			jwtKeysetSpec: "",
			wantErr:       true,
		},
		{
			name:          "strict rejects default jwt secret without keyset",
			strict:        true,
			databaseURL:   "postgres://x",
			tlsEnabled:    true,
			jwtSecret:     "dev-insecure-change-me",
			jwtKeysetSpec: "",
			wantErr:       true,
		},
		{
			name:          "strict allows keyset with default single secret value",
			strict:        true,
			databaseURL:   "postgres://x",
			tlsEnabled:    true,
			jwtSecret:     "dev-insecure-change-me",
			jwtKeysetSpec: "k1:rotated-secret",
			wantErr:       false,
		},
		{
			name:          "strict valid config",
			strict:        true,
			databaseURL:   "postgres://x",
			tlsEnabled:    true,
			jwtSecret:     "prod-secret",
			jwtKeysetSpec: "",
			wantErr:       false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateProductionRuntime(tc.strict, tc.databaseURL, tc.tlsEnabled, tc.jwtSecret, tc.jwtKeysetSpec)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateProductionRuntime() err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
