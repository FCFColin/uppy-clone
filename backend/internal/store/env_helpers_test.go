package store

import (
	"os"
	"testing"
	"time"
)

func TestGetEnvInt(t *testing.T) {
	t.Run("returns default when env not set", func(t *testing.T) {
		os.Unsetenv("TEST_STORE_INT")
		got := getEnvInt("TEST_STORE_INT", 42)
		if got != 42 {
			t.Errorf("getEnvInt = %d, want %d", got, 42)
		}
	})

	t.Run("returns default when env is empty", func(t *testing.T) {
		os.Setenv("TEST_STORE_INT", "")
		defer os.Unsetenv("TEST_STORE_INT")
		got := getEnvInt("TEST_STORE_INT", 10)
		if got != 10 {
			t.Errorf("getEnvInt = %d, want %d", got, 10)
		}
	})

	t.Run("parses valid int", func(t *testing.T) {
		os.Setenv("TEST_STORE_INT", "50")
		defer os.Unsetenv("TEST_STORE_INT")
		got := getEnvInt("TEST_STORE_INT", 10)
		if got != 50 {
			t.Errorf("getEnvInt = %d, want %d", got, 50)
		}
	})

	t.Run("returns default for zero", func(t *testing.T) {
		os.Setenv("TEST_STORE_INT", "0")
		defer os.Unsetenv("TEST_STORE_INT")
		got := getEnvInt("TEST_STORE_INT", 10)
		if got != 10 {
			t.Errorf("getEnvInt = %d, want %d", got, 10)
		}
	})

	t.Run("returns default for negative", func(t *testing.T) {
		os.Setenv("TEST_STORE_INT", "-5")
		defer os.Unsetenv("TEST_STORE_INT")
		got := getEnvInt("TEST_STORE_INT", 10)
		if got != 10 {
			t.Errorf("getEnvInt = %d, want %d", got, 10)
		}
	})

	t.Run("returns default for invalid string", func(t *testing.T) {
		os.Setenv("TEST_STORE_INT", "not-a-number")
		defer os.Unsetenv("TEST_STORE_INT")
		got := getEnvInt("TEST_STORE_INT", 10)
		if got != 10 {
			t.Errorf("getEnvInt = %d, want %d", got, 10)
		}
	})
}

func TestGetEnvDuration(t *testing.T) {
	t.Run("returns default when env not set", func(t *testing.T) {
		os.Unsetenv("TEST_STORE_DUR")
		got := getEnvDuration("TEST_STORE_DUR", 5*time.Second)
		if got != 5*time.Second {
			t.Errorf("getEnvDuration = %v, want %v", got, 5*time.Second)
		}
	})

	t.Run("returns default when env is empty", func(t *testing.T) {
		os.Setenv("TEST_STORE_DUR", "")
		defer os.Unsetenv("TEST_STORE_DUR")
		got := getEnvDuration("TEST_STORE_DUR", 3*time.Second)
		if got != 3*time.Second {
			t.Errorf("getEnvDuration = %v, want %v", got, 3*time.Second)
		}
	})

	t.Run("parses valid duration", func(t *testing.T) {
		os.Setenv("TEST_STORE_DUR", "30s")
		defer os.Unsetenv("TEST_STORE_DUR")
		got := getEnvDuration("TEST_STORE_DUR", 5*time.Second)
		if got != 30*time.Second {
			t.Errorf("getEnvDuration = %v, want %v", got, 30*time.Second)
		}
	})

	t.Run("parses minutes", func(t *testing.T) {
		os.Setenv("TEST_STORE_DUR", "5m")
		defer os.Unsetenv("TEST_STORE_DUR")
		got := getEnvDuration("TEST_STORE_DUR", 1*time.Second)
		if got != 5*time.Minute {
			t.Errorf("getEnvDuration = %v, want %v", got, 5*time.Minute)
		}
	})

	t.Run("returns default for zero duration", func(t *testing.T) {
		os.Setenv("TEST_STORE_DUR", "0s")
		defer os.Unsetenv("TEST_STORE_DUR")
		got := getEnvDuration("TEST_STORE_DUR", 10*time.Second)
		if got != 10*time.Second {
			t.Errorf("getEnvDuration = %v, want %v", got, 10*time.Second)
		}
	})

	t.Run("returns default for negative duration", func(t *testing.T) {
		os.Setenv("TEST_STORE_DUR", "-5s")
		defer os.Unsetenv("TEST_STORE_DUR")
		got := getEnvDuration("TEST_STORE_DUR", 10*time.Second)
		if got != 10*time.Second {
			t.Errorf("getEnvDuration = %v, want %v", got, 10*time.Second)
		}
	})

	t.Run("returns default for invalid duration", func(t *testing.T) {
		os.Setenv("TEST_STORE_DUR", "not-a-duration")
		defer os.Unsetenv("TEST_STORE_DUR")
		got := getEnvDuration("TEST_STORE_DUR", 10*time.Second)
		if got != 10*time.Second {
			t.Errorf("getEnvDuration = %v, want %v", got, 10*time.Second)
		}
	})
}
