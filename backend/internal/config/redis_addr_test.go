package config

import "testing"

func TestParseRedisURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw      string
		wantAddr string
		wantPass string
		wantErr  bool
	}{
		{"", "", "", true},
		{"localhost:6379", "localhost:6379", "", false},
		{"redis:6379", "redis:6379", "", false},
		{"redis://:secret@redis:6379", "redis:6379", "secret", false},
		{"redis://redis:6379/0", "redis:6379", "", false},
		{"rediss://:pass@secure:6380/1", "secure:6380", "pass", false},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()
			got, err := ParseRedisURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRedisURL: %v", err)
			}
			if got.Addr != tt.wantAddr || got.Password != tt.wantPass {
				t.Fatalf("got %+v want addr=%q pass=%q", got, tt.wantAddr, tt.wantPass)
			}
		})
	}
}

func TestParseRedisURL_MissingHost(t *testing.T) {
	_, err := ParseRedisURL("redis://")
	if err == nil {
		t.Fatal("expected error for redis URL without host")
	}
}

func TestParseRedisURL_InvalidURL(t *testing.T) {
	_, err := ParseRedisURL("redis://%zz")
	if err == nil {
		t.Fatal("expected parse error")
	}
}
