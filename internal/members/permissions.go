package members

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleAgent  Role = "agent"
	RoleViewer Role = "viewer"
)

type Permission string

const (
	PermissionMembersManage       Permission = "members:manage"
	PermissionMembersRead         Permission = "members:read"
	PermissionAccountsManage      Permission = "accounts:manage"
	PermissionAccountsRead        Permission = "accounts:read"
	PermissionKnowledgeManage     Permission = "knowledge:manage"
	PermissionKnowledgeRead       Permission = "knowledge:read"
	PermissionModelsManage        Permission = "models:manage"
	PermissionConversationsManage Permission = "conversations:manage"
	PermissionConversationsRead   Permission = "conversations:read"
	PermissionCustomersRead       Permission = "customers:read"
	PermissionSettingsManage      Permission = "settings:manage"
	PermissionMetricsRead         Permission = "metrics:read"
	PermissionAlertsRead          Permission = "alerts:read"
	PermissionAuditRead           Permission = "audit:read"
)

var Permissions = []Permission{
	PermissionMembersManage, PermissionMembersRead,
	PermissionAccountsManage, PermissionAccountsRead,
	PermissionKnowledgeManage, PermissionKnowledgeRead,
	PermissionModelsManage,
	PermissionConversationsManage, PermissionConversationsRead,
	PermissionCustomersRead, PermissionSettingsManage,
	PermissionMetricsRead, PermissionAlertsRead, PermissionAuditRead,
}

var rolePermissions = map[Role]map[Permission]struct{}{
	RoleOwner: permissionSet(Permissions...),
	RoleAdmin: permissionSet(
		PermissionMembersRead,
		PermissionAccountsManage, PermissionAccountsRead,
		PermissionKnowledgeManage, PermissionKnowledgeRead,
		PermissionModelsManage,
		PermissionConversationsManage, PermissionConversationsRead,
		PermissionCustomersRead, PermissionSettingsManage,
		PermissionMetricsRead, PermissionAlertsRead, PermissionAuditRead,
	),
	RoleAgent: permissionSet(
		PermissionAccountsRead, PermissionKnowledgeRead,
		PermissionConversationsManage, PermissionConversationsRead,
		PermissionCustomersRead, PermissionMetricsRead, PermissionAlertsRead,
	),
	RoleViewer: permissionSet(
		PermissionAccountsRead, PermissionMetricsRead, PermissionAlertsRead, PermissionAuditRead,
	),
}

func HasPermission(role Role, permission Permission) bool {
	_, ok := rolePermissions[role][permission]
	return ok
}

func PermissionsFor(role Role) []Permission {
	result := make([]Permission, 0, len(rolePermissions[role]))
	for _, permission := range Permissions {
		if HasPermission(role, permission) {
			result = append(result, permission)
		}
	}
	return result
}

func ValidRole(role Role) bool {
	_, ok := rolePermissions[role]
	return ok
}

func permissionSet(permissions ...Permission) map[Permission]struct{} {
	result := make(map[Permission]struct{}, len(permissions))
	for _, permission := range permissions {
		result[permission] = struct{}{}
	}
	return result
}
