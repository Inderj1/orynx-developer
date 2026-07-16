package handler

import "testing"

// The dev verification code is a security-sensitive login bypass. It must be
// impossible to activate without an explicit MULTICA_DEV_MODE opt-in AND a
// non-production APP_ENV — a misconfigured "staging"/unset deploy must not
// silently accept a static code.
func TestDevVerificationCodeGate(t *testing.T) {
	const code = "424242"
	cases := []struct {
		name    string
		appEnv  string
		devMode string
		devCode string
		input   string
		want    bool
	}{
		{"dev mode off rejects even the correct code", "", "", code, code, false},
		{"dev mode on + non-prod accepts correct code", "development", "1", code, code, true},
		{"dev mode 'true' also accepted", "development", "true", code, code, true},
		{"production rejects even with dev mode on", "production", "1", code, code, false},
		{"dev mode on rejects a wrong code", "development", "1", code, "000000", false},
		{"dev mode on but no dev code configured rejects", "development", "1", "", code, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APP_ENV", tc.appEnv)
			t.Setenv("MULTICA_DEV_MODE", tc.devMode)
			t.Setenv(devVerificationCodeEnv, tc.devCode)
			if got := isDevVerificationCode(tc.input); got != tc.want {
				t.Errorf("isDevVerificationCode(%q) = %v, want %v (APP_ENV=%q DEV_MODE=%q DEV_CODE=%q)",
					tc.input, got, tc.want, tc.appEnv, tc.devMode, tc.devCode)
			}
		})
	}
}
