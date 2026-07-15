package config

import "testing"

func TestParseRedisURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw      string
		wantAddr string
		wantPass string
		wantDB   int
		wantErr  bool
	}{
		{"", "", "", 0, true},
		{"localhost:6379", "localhost:6379", "", 0, false},
		{"redis:6379", "redis:6379", "", 0, false},
		{"redis://:secret@redis:6379", "redis:6379", "secret", 0, false},
		{"redis://redis:6379/0", "redis:6379", "", 0, false},
		{"redis://redis:6379/2", "redis:6379", "", 2, false},
		{"rediss://:pass@secure:6380/1", "secure:6380", "pass", 1, false},
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
			if got.Addr != tt.wantAddr || got.Password != tt.wantPass || got.DB != tt.wantDB {
				t.Fatalf("got %+v want addr=%q pass=%q db=%d", got, tt.wantAddr, tt.wantPass, tt.wantDB)
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

func TestParseRedisURL_InvalidDB(t *testing.T) {
	_, err := ParseRedisURL("redis://localhost:6379/abc")
	if err == nil {
		t.Fatal("expected error for invalid DB number")
	}
}
