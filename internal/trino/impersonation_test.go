package trino

import (
	"context"
	"testing"
)

func TestWithImpersonatedUser(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantOk   bool
	}{
		{
			name:     "valid username",
			username: "alice@example.com",
			wantOk:   true,
		},
		{
			name:     "empty username",
			username: "",
			wantOk:   true, // empty string is still stored
		},
		{
			name:     "username with special characters",
			username: "user+test@example.com",
			wantOk:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = WithImpersonatedUser(ctx, tt.username)

			got, ok := GetImpersonatedUser(ctx)
			if ok != tt.wantOk {
				t.Errorf("GetImpersonatedUser() ok = %v, want %v", ok, tt.wantOk)
			}
			if got != tt.username {
				t.Errorf("GetImpersonatedUser() = %v, want %v", got, tt.username)
			}
		})
	}
}

func TestGetImpersonatedUser_NotSet(t *testing.T) {
	ctx := context.Background()
	got, ok := GetImpersonatedUser(ctx)
	if ok {
		t.Errorf("GetImpersonatedUser() ok = true, want false")
	}
	if got != "" {
		t.Errorf("GetImpersonatedUser() = %v, want empty string", got)
	}
}

func TestImpersonationContextPreservation(t *testing.T) {
	// Test that impersonation context is preserved through context.WithTimeout
	ctx := context.Background()
	ctx = WithImpersonatedUser(ctx, "test-user")

	// Simulate what happens in ExecuteQueryWithContext
	timeoutCtx, cancel := context.WithTimeout(ctx, 0)
	defer cancel()

	got, ok := GetImpersonatedUser(timeoutCtx)
	if !ok {
		t.Error("GetImpersonatedUser() from timeout context ok = false, want true")
	}
	if got != "test-user" {
		t.Errorf("GetImpersonatedUser() from timeout context = %v, want test-user", got)
	}
}
