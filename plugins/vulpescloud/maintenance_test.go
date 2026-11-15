package vulpescloud

import "testing"

func TestNodeMaintenanceJoinPermissionPrecedence(t *testing.T) {
	globalPermission := "maintenance.join"
	nodePermission := "node.join"
	config := &NodeMaintenanceConfig{
		JoinPermission: &globalPermission,
		Nodes: map[string]NodeMaintenanceEntry{
			"node-a": {Maintenance: true, JoinPermission: &nodePermission},
			"node-b": {Maintenance: true},
		},
	}

	if permission, ok := config.ResolveJoinPermission("node-a"); !ok || permission != nodePermission {
		t.Fatalf("expected node permission %q, got %q (enabled=%t)", nodePermission, permission, ok)
	}
	if permission, ok := config.ResolveJoinPermission("node-b"); !ok || permission != globalPermission {
		t.Fatalf("expected global permission %q, got %q (enabled=%t)", globalPermission, permission, ok)
	}
}

func TestNodeMaintenanceEmptyNodePermissionDisablesGlobal(t *testing.T) {
	globalPermission := "maintenance.join"
	noPermission := ""
	config := &NodeMaintenanceConfig{
		JoinPermission: &globalPermission,
		Nodes: map[string]NodeMaintenanceEntry{
			"node-a": {JoinPermission: &noPermission},
		},
	}

	if permission, ok := config.ResolveJoinPermission("node-a"); ok {
		t.Fatalf("expected no permission requirement, got %q", permission)
	}
}
