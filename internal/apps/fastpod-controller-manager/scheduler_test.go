package fastpodcontrollermanager

import (
	"fmt"
	"testing"

	"github.com/KontonGu/FaST-GShare/pkg/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func TestFindBestNode(t *testing.T) {
	// Mock data setup
	mockLister := &MockNodeLister{
		Nodes: []string{"node1"},
	}

	ctr := &Controller{
		nodesLister: mockLister,
	}

	req := &ResourceRequest{
		AllocationType: types.AllocationTypeExclusive,
		Memory:         8 * 1024 * 1024 * 1024, // 8GB
		SMRequest:      intPtr(40),
	}

	_, err := ctr.FindBestNode(req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

}

// MockNodeLister is a mock implementation of a NodeLister
// for testing purposes.
type MockNodeLister struct {
	Nodes []string
}

// Updated Get method to return the correct type
func (m *MockNodeLister) Get(name string) (*v1.Node, error) {
	for _, nodeName := range m.Nodes {
		if nodeName == name {
			return &v1.Node{}, nil // Adjusted to return a valid v1.Node instance
		}
	}
	return nil, fmt.Errorf("node not found")
}

// Removed unused variable nodeName in the loop
func (m *MockNodeLister) List(selector labels.Selector) ([]*v1.Node, error) {
	var nodeList []*v1.Node
	for range m.Nodes {
		nodeList = append(nodeList, &v1.Node{}) // Adjusted to match v1.Node struct definition
	}
	return nodeList, nil
}

func intPtr(i int) *int {
	return &i
}
