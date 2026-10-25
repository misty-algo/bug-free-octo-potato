package service

import (
	"strings"

	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// 角色常量。审核与安全角色只出现在服务端放行判断里，
// 司机只读由服务端权限兜底，不依赖前端隐藏入口。
const (
	RoleDriver        = "driver"
	RoleAuditor       = "auditor"
	RoleSafetyHandler = "safety_handler"
)

// requireRole 由服务端兜底做角色校验，前端字段级提示只是补充，不能替代。
func requireRole(principal authn.Principal, roles ...string) error {
	if strings.TrimSpace(principal.UserID) == "" {
		return apperrors.Unauthorized("UNAUTHENTICATED", "a valid principal is required")
	}
	for _, role := range roles {
		if strings.EqualFold(principal.Role, role) {
			return nil
		}
	}
	return apperrors.Forbidden("FORBIDDEN", strings.Join(roles, "/")+" role is required")
}

func hasRole(principal authn.Principal, role string) bool {
	return strings.EqualFold(principal.Role, role)
}
