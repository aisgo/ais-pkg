package mysql

import (
	"strings"
	"testing"
)

func TestSanitizeMySQLDSN(t *testing.T) {
	tests := []struct {
		name          string
		dsn           string
		wantContains  string
		wantAbsent    string
	}{
		{
			name:         "password is redacted",
			dsn:          "user:supersecret@tcp(127.0.0.1:3306)/mydb?charset=utf8mb4&parseTime=True",
			wantContains: "***",
			wantAbsent:   "supersecret",
		},
		{
			name:         "no password is safe",
			dsn:          "user@tcp(127.0.0.1:3306)/mydb",
			wantContains: "user",
			wantAbsent:   "supersecret",
		},
		{
			name:         "invalid dsn returns redacted",
			dsn:          "not-a-valid-dsn!!!",
			wantContains: "[redacted]",
			wantAbsent:   "not-a-valid-dsn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeMySQLDSN(tt.dsn)
			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("sanitizeMySQLDSN() = %q, want it to contain %q", got, tt.wantContains)
			}
			if tt.wantAbsent != "" && strings.Contains(got, tt.wantAbsent) {
				t.Errorf("sanitizeMySQLDSN() = %q, must not contain %q", got, tt.wantAbsent)
			}
		})
	}
}

func TestResolveParseTime(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			name: "default enabled",
			cfg:  Config{},
			want: true,
		},
		{
			name: "explicit true stays enabled",
			cfg: Config{
				ParseTime: true,
			},
			want: true,
		},
		{
			name: "disable flag turns it off",
			cfg: Config{
				DisableParseTime: true,
			},
			want: false,
		},
		{
			name: "disable flag wins",
			cfg: Config{
				ParseTime:        true,
				DisableParseTime: true,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveParseTime(tt.cfg); got != tt.want {
				t.Fatalf("resolveParseTime() = %v, want %v", got, tt.want)
			}
		})
	}
}
