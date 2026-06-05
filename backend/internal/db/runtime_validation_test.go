package db

import "testing"

func TestRoleBypassesRLS(t *testing.T) {
	tests := []struct {
		name string
		role runtimeRoleInfo
		want bool
	}{
		{name: "plain app role", role: runtimeRoleInfo{User: "app"}, want: false},
		{name: "superuser", role: runtimeRoleInfo{User: "postgres", IsSuperuser: true}, want: true},
		{name: "bypassrls", role: runtimeRoleInfo{User: "app_admin", BypassRLS: true}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := roleBypassesRLS(tt.role); got != tt.want {
				t.Fatalf("roleBypassesRLS() = %v, want %v", got, tt.want)
			}
		})
	}
}
