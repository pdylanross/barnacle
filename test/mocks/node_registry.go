package mocks

import (
	"context"

	"github.com/pdylanross/barnacle/internal/node"
)

// NodeRegistry is a mock implementation of coordinator.NodeRegistryProvider for testing.
type NodeRegistry struct {
	nodeID     string
	nodeInfo   *node.Info
	otherNodes []*node.Info
	otherErr   error
}

// NewNodeRegistry creates a new mock node registry with the given node ID and info.
func NewNodeRegistry(nodeID string, nodeInfo *node.Info) *NodeRegistry {
	return &NodeRegistry{
		nodeID:   nodeID,
		nodeInfo: nodeInfo,
	}
}

// SetOtherNodes configures the nodes returned by ListOtherNodes.
func (m *NodeRegistry) SetOtherNodes(nodes []*node.Info) {
	m.otherNodes = nodes
}

// SetOtherNodesError configures ListOtherNodes to return an error.
func (m *NodeRegistry) SetOtherNodesError(err error) {
	m.otherErr = err
}

// NodeID returns the configured node ID.
func (m *NodeRegistry) NodeID() string {
	return m.nodeID
}

// GetNodeInfo returns the configured node info.
func (m *NodeRegistry) GetNodeInfo() *node.Info {
	return m.nodeInfo
}

// ListOtherNodes returns the configured other nodes or error.
func (m *NodeRegistry) ListOtherNodes(_ context.Context) ([]*node.Info, error) {
	if m.otherErr != nil {
		return nil, m.otherErr
	}
	return m.otherNodes, nil
}
