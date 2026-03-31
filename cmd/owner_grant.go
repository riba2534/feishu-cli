package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
)

// grantOwnerPermission automatically grants permission to the configured owner
// after document creation. It reads owner_open_id (priority) or owner_email from
// config, adds full_access permission, and optionally transfers ownership.
// If no owner is configured, it silently returns nil.
func grantOwnerPermission(docToken, docType string) error {
	cfg := config.Get()
	memberType, memberID := cfg.GetOwner()
	if memberType == "" {
		return nil
	}

	// Add full_access permission
	member := client.PermissionMember{
		MemberType: memberType,
		MemberID:   memberID,
		Perm:       "full_access",
	}
	if err := client.AddPermission(docToken, docType, member, true); err != nil {
		return fmt.Errorf("自动授权失败: %w", err)
	}
	fmt.Printf("  已授权 %s(%s) full_access 权限\n", memberID, memberType)

	// Transfer ownership if configured
	if cfg.TransferOwnership {
		if err := client.TransferOwnership(docToken, docType, memberType, memberID, true, false, false, "full_access"); err != nil {
			return fmt.Errorf("转移所有权失败: %w", err)
		}
		fmt.Printf("  已转移所有权给 %s(%s)\n", memberID, memberType)
	}

	return nil
}
