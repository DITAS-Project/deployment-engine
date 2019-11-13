package ansible

import (
	"deployment-engine/model"
	"fmt"
	"testing"

	"github.com/go-test/deep"
)

func buildNode(name, ip, user, role, expectedRole string) (model.NodeInfo, InventoryHost) {
	node := model.NodeInfo{
		Hostname: name,
		IP:       ip,
		Username: user,
	}

	expected := InventoryHost{
		Name: name,
		Vars: map[string]string{
			ansibleHostProperty: ip,
			ansibleUserProperty: user,
		},
	}

	if role != "" {
		node.Role = role
	}

	if expectedRole != "" {
		expected.Vars[kuberneterRoleProperty] = expectedRole
	}

	return node, expected
}

func buildInfra(numNodes int, isk8s bool) (model.InfrastructureDeploymentInfo, map[string]InventoryHost, Inventory) {
	totalNodes := make([]model.NodeInfo, numNodes)
	totalHosts := make([]InventoryHost, numNodes)
	nodes := make(map[string]model.NodeInfo)
	expected := make(map[string]InventoryHost)
	for i := 0; i < numNodes; i++ {
		role := "master"
		expectedRole := ""
		if isk8s {
			expectedRole = "master"
		}
		if i > numNodes/2 {
			role = "slave"
			if isk8s {
				expectedRole = "node"
			}
		}
		name := fmt.Sprintf("node%d", i)
		nodes[name], expected[name] = buildNode(
			name,
			fmt.Sprintf("192.168.1.%d", i),
			"clouduser", role, expectedRole)
		totalNodes[i] = nodes[name]
		totalHosts[i] = expected[name]
	}

	infra := model.InfrastructureDeploymentInfo{
		Nodes: map[string][]model.NodeInfo{
			"master": totalNodes[:numNodes/2],
			"slave":  totalNodes[numNodes/2:],
		},
	}

	invExpected := Inventory{
		Hosts: totalHosts,
	}

	if isk8s {
		invExpected.Groups = make([]InventoryGroup, 0, len(infra.Nodes))
		for group, hosts := range infra.Nodes {
			currentGroup := InventoryGroup{
				Name:  group,
				Hosts: make([]string, len(hosts)),
			}
			for i, host := range hosts {
				currentGroup.Hosts[i] = host.Hostname
			}
			invExpected.Groups = append(invExpected.Groups, currentGroup)
		}
	}

	return infra, expected, invExpected
}

func testHostEquality(t *testing.T, host, expected InventoryHost) {
	if diff := deep.Equal(host, expected); diff != nil {
		t.Fatal(diff)
	}
}

func testNodeEquality(t *testing.T, name, ip, user, role, expectedRole string, transformer func(model.NodeInfo) InventoryHost) {
	node, expected := buildNode(name, ip, user, role, expectedRole)
	host := transformer(node)
	testHostEquality(t, host, expected)
}

func TestInventoryNode(t *testing.T) {
	testNodeEquality(t, "test-node", "127.0.0.1", "clouduser", "", "", DefaultInventoryHost)
}

func TestKubernetesInventoryNode(t *testing.T) {
	testNodeEquality(t, "test-node", "127.0.0.1", "clouduser", "master", "master", DefaultKubernetesInventoryHost)
	testNodeEquality(t, "test-node", "127.0.0.1", "clouduser", "mAstEr", "master", DefaultKubernetesInventoryHost)
	testNodeEquality(t, "test-node", "127.0.0.1", "clouduser", "slave", "node", DefaultKubernetesInventoryHost)
	testNodeEquality(t, "test-node", "127.0.0.1", "clouduser", "OtherRole", "node", DefaultKubernetesInventoryHost)
}

func testBasicInventory(t *testing.T, numNodes int, isk8s bool) (model.InfrastructureDeploymentInfo, map[string]InventoryHost, Inventory, Inventory) {
	infra, expected, invExpected := buildInfra(10, isk8s)

	var inventory Inventory
	if isk8s {
		inventory = DefaultKubernetesInventory(infra)
	} else {
		inventory = DefaultAllInventory(infra)
	}

	if len(invExpected.Hosts) != 10 {
		t.Fatalf("Different length of inventory found %d vs expected %d", 10, len(invExpected.Hosts))
	}

	for _, host := range inventory.Hosts {
		testHostEquality(t, host, expected[host.Name])
	}

	return infra, expected, invExpected, inventory
}

func TestDefaultInventory(t *testing.T) {
	testBasicInventory(t, 10, false)
}

func TestKubernetesInventory(t *testing.T) {
	infra, _, invExpected, inventory := testBasicInventory(t, 10, true)

	if len(inventory.Groups) != len(invExpected.Groups) {
		t.Fatalf("Expected %d groups but found %d", len(invExpected.Groups), len(inventory.Groups))
	}

	for _, group := range inventory.Groups {
		var nodes []model.NodeInfo
		switch group.Name {
		case "master":
			nodes = infra.Nodes["master"]
		case "slave":
			nodes = infra.Nodes["slave"]
		default:
			t.Fatalf("Expected role to be 'master' or 'slave' but found '%s'", group.Name)
		}

		if nodes == nil {
			t.Fatal("Didn't find expected nodes to compate")
		}

		if len(group.Hosts) != len(nodes) {
			t.Fatalf("Different host number found in group %s. Expected %d but found %d", group.Name, len(nodes), len(group.Hosts))
		}

		for _, host := range nodes {
			found := false
			for i := 0; i < len(nodes) && !found; i++ {
				if group.Hosts[i] == host.Hostname {
					found = true
				}
			}
			if !found {
				t.Fatalf("Expected to find host %s in group %s but not found", host.Hostname, group.Name)
			}
		}
	}
}
