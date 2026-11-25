//go:build !mygo_noinspector

package ui

import "testing"

// TestInspectorRoleNames checks that the inspector names the roles of
// deleted and inserted content tab panels and landmarks as Chrome does.
func TestInspectorRoleNames(t *testing.T) {
	for r, want := range map[Role]string{RoleDeletion: "deletion", RoleInsertion: "insertion", RoleTabPanel: "tabpanel",
		RoleMain: "main", RoleBanner: "banner", RoleContentInfo: "contentinfo", RoleComplementary: "complementary",
		RoleForm: "form"} {
		e := &node{role: r}
		if got, seen := accessibleRole(e); got != want || !seen {
			t.Errorf("role %d: %q, %v", r, got, seen)
		}
		if roleName(r) == "" {
			t.Errorf("role %d has no name", r)
		}
	}
}
