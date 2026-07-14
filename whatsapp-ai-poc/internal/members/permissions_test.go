package members_test

import (
	"testing"

	"whatsapp-ai-poc/internal/members"
)

func TestCompletePermissionMatrix(t *testing.T) {
	all := permissionSet(members.Permissions...)
	expected := map[members.Role]map[members.Permission]bool{
		members.RoleOwner: all,
		members.RoleAdmin: permissionSet(
			members.PermissionMembersRead,
			members.PermissionAccountsManage, members.PermissionAccountsRead,
			members.PermissionKnowledgeManage, members.PermissionKnowledgeRead,
			members.PermissionModelsManage,
			members.PermissionConversationsManage, members.PermissionConversationsRead,
			members.PermissionCustomersRead, members.PermissionSettingsManage,
			members.PermissionMetricsRead, members.PermissionAlertsRead, members.PermissionAuditRead,
		),
		members.RoleAgent: permissionSet(
			members.PermissionAccountsRead, members.PermissionKnowledgeRead,
			members.PermissionConversationsManage, members.PermissionConversationsRead,
			members.PermissionCustomersRead, members.PermissionMetricsRead, members.PermissionAlertsRead,
		),
		members.RoleViewer: permissionSet(
			members.PermissionAccountsRead, members.PermissionMetricsRead,
			members.PermissionAlertsRead, members.PermissionAuditRead,
		),
	}

	for _, role := range []members.Role{members.RoleOwner, members.RoleAdmin, members.RoleAgent, members.RoleViewer} {
		listed := permissionSet(members.PermissionsFor(role)...)
		for _, permission := range members.Permissions {
			if got, want := members.HasPermission(role, permission), expected[role][permission]; got != want {
				t.Errorf("HasPermission(%q, %q)=%v, want %v", role, permission, got, want)
			}
			if listed[permission] != expected[role][permission] {
				t.Errorf("PermissionsFor(%q) membership for %q=%v, want %v", role, permission, listed[permission], expected[role][permission])
			}
		}
	}
	if members.HasPermission("unknown", members.PermissionMetricsRead) {
		t.Fatal("unknown role received a permission")
	}
	if len(members.PermissionsFor("unknown")) != 0 {
		t.Fatal("unknown role received a permission list")
	}
}

func permissionSet(permissions ...members.Permission) map[members.Permission]bool {
	result := make(map[members.Permission]bool, len(permissions))
	for _, permission := range permissions {
		result[permission] = true
	}
	return result
}
