package db

import (
	"context"
	"fmt"
	"strings"

	"caseagent/internal/config"

	"github.com/uptrace/bun"
)

type runtimeRoleInfo struct {
	User        string `bun:"user"`
	IsSuperuser bool   `bun:"is_superuser"`
	BypassRLS   bool   `bun:"bypassrls"`
}

func validateRuntimeConfig(ctx context.Context, bunDB *bun.DB, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("runtime validation: config is nil")
	}
	rejectBypass := !strings.EqualFold(strings.TrimSpace(cfg.Server.Mode), "debug")
	if !rejectBypass {
		return nil
	}

	role, err := currentRuntimeRole(ctx, bunDB)
	if err != nil {
		return err
	}
	if roleBypassesRLS(role) {
		return fmt.Errorf(
			"runtime validation: database role %q is superuser or BYPASSRLS; use a NOBYPASSRLS role outside debug mode",
			role.User,
		)
	}
	return nil
}

func currentRuntimeRole(ctx context.Context, bunDB *bun.DB) (runtimeRoleInfo, error) {
	var row runtimeRoleInfo
	if err := bunDB.NewRaw(`
		SELECT current_user AS "user",
		       rolsuper AS is_superuser,
		       rolbypassrls AS bypassrls
		FROM pg_roles
		WHERE rolname = current_user
	`).Scan(ctx, &row); err != nil {
		return runtimeRoleInfo{}, fmt.Errorf("runtime validation: inspect database role: %w", err)
	}
	return row, nil
}

func roleBypassesRLS(role runtimeRoleInfo) bool {
	return role.IsSuperuser || role.BypassRLS
}
