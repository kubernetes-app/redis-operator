package resources

import (
	"fmt"
	"strings"
)

const (
	// DefaultRedisPort define the default Redis Port
	DefaultRedisPort = "6379"
	// RedisMasterRole redis role master
	RedisMasterRole = "master"
	// RedisSlaveRole redis role slave
	RedisSlaveRole = "slave"
)

// Node Represent a Redis Node
type Node struct {
	ID             string
	IP             string
	Port           string
	Role           string
	LinkState      string
	MasterReferent string
	Slot           string
}

// Nodes represent a Node slice
type Nodes []*Node

// FindNodeFunc function for finding a Node
// it is use as input for GetNodeByFunc and GetNodesByFunc
type FindNodeFunc func(node *Node) bool

// NewDefaultNode builds and returns new defaultNode instance
func NewDefaultNode() *Node {
	return &Node{
		Port: DefaultRedisPort,
	}
}

// SetRole from a flags string list set the Node's role
func (n *Node) SetRole(flags string) {
	n.Role = "" // reset value before setting the new one
	vals := strings.Split(flags, ",")
	for _, val := range vals {
		switch val {
		case RedisMasterRole:
			n.Role = RedisMasterRole
		case RedisSlaveRole:
			n.Role = RedisSlaveRole
		}
	}
}

// GetNodesByFunc returns first node found by the FindNodeFunc
func (n Nodes) GetNodesByFunc(f FindNodeFunc) (Nodes, error) {
	nodes := Nodes{}
	for _, node := range n {
		if f(node) {
			nodes = append(nodes, node)
		}
	}
	if len(nodes) == 0 {
		return nodes, fmt.Errorf("node not found")
	}
	return nodes, nil
}

// SetReferentMaster set the redis node parent referent
func (n *Node) SetReferentMaster(ref string) {
	n.MasterReferent = ""
	if ref == "-" {
		return
	}
	n.MasterReferent = ref
}

// GetNodeByID returns a Redis Node by its ID
// if not present in the Nodes slice return an error
func (n Nodes) GetNodeByID(id string) (*Node, error) {
	for _, node := range n {
		if node.ID == id {
			return node, nil
		}
	}

	return nil, fmt.Errorf("node not found")
}

// CountByFunc gives the number elements of NodeSlice that return true for the passed func.
func (n Nodes) CountByFunc(fn func(*Node) bool) (result int) {
	for _, v := range n {
		if fn(v) {
			result++
		}
	}
	return
}

// FilterByFunc remove a node from a slice by node ID and returns the slice. If not found, fail silently. Value must be unique
func (n Nodes) FilterByFunc(fn func(*Node) bool) Nodes {
	newSlice := Nodes{}
	for _, node := range n {
		if fn(node) {
			newSlice = append(newSlice, node)
		}
	}
	return newSlice
}

func (n Nodes) GetClusterFromNodeIds() string {
	filterNodes := n.FilterByFunc(IsMasterWithSlot)
	var ids []string
	for _, n := range filterNodes {
		ids = append(ids, n.ID)
	}
	return strings.Join(ids, ",")
}

func (n Nodes) GetClusterToNodeID() string {
	filterNodes := n.FilterByFunc(IsMasterWithNoSlot)
	return filterNodes[0].ID
}

var AllNodes = func(n *Node) bool {
	return true
}

var IsMaster = func(n *Node) bool {
	return n.Role == RedisMasterRole
}

var IsSlave = func(n *Node) bool {
	return n.Role == RedisSlaveRole
}

// IsMasterWithNoSlot anonymous function for searching Master Node with no slot
var IsMasterWithNoSlot = func(n *Node) bool {
	if (n.Role == RedisMasterRole) && (n.Slot == "") {
		return true
	}
	return false
}

// IsMasterWithSlot anonymous function for searching Master Node withslot
var IsMasterWithSlot = func(n *Node) bool {
	if (n.Role == RedisMasterRole) && (n.Slot != "") {
		return true
	}
	return false
}
