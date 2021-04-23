/*
Copyright 2021 kubernetes-app Solutions.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package redis

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

const (
	// RedisLinkStateConnected redis connection status connected
	RedisLinkStateConnected = "connected"
	// RedisLinkStateDisconnected redis connection status disconnected
	RedisLinkStateDisconnected = "disconnected"
)

const (
	// NodeStatusPFail Node is in PFAIL state. Not reachable for the node you are contacting, but still logically reachable
	NodeStatusPFail = "fail?"
	// NodeStatusFail Node is in FAIL state. It was not reachable for multiple nodes that promoted the PFAIL state to FAIL
	NodeStatusFail = "fail"
	// NodeStatusHandshake Untrusted node, we are handshaking.
	NodeStatusHandshake = "handshake"
	// NodeStatusNoAddr No address known for this node
	NodeStatusNoAddr = "noaddr"
	// NodeStatusNoFlags no flags at all
	NodeStatusNoFlags = "noflags"
)

// Node Represent a Redis Node
type Node struct {
	ID              string
	IP              string
	Port            string
	Role            string
	LinkState       string
	MasterReferent  string
	FailStatus      []string
	PingSent        int64
	PongRecv        int64
	ConfigEpoch     int64
	Slots           []Slot
	MigratingSlots  map[Slot]string
	ImportingSlots  map[Slot]string
	ServerStartTime time.Time

	Pod *corev1.Pod
}

// Nodes represent a Node slice
type Nodes []*Node

// FindNodeFunc function for finding a Node
// it is use as input for GetNodeByFunc and GetNodesByFunc
type FindNodeFunc func(node *Node) bool

func (n Nodes) String() string {
	stringer := []Stringer{}
	for _, node := range n {
		stringer = append(stringer, node)
	}

	return SliceJoin(stringer, ",")
}

// NewDefaultNode builds and returns new defaultNode instance
func NewDefaultNode() *Node {
	return &Node{
		Port:           DefaultRedisPort,
		Slots:          []Slot{},
		MigratingSlots: map[Slot]string{},
		ImportingSlots: map[Slot]string{},
	}
}

// NewNode builds and returns new Node instance
func NewNode(id, ip string, pod *corev1.Pod) *Node {
	node := NewDefaultNode()
	node.ID = id
	node.IP = ip
	node.Pod = pod

	return node
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

// GetRole return the Redis role
func (n *Node) GetRole() string {
	switch n.Role {
	case RedisMasterRole:
		return RedisMasterRole
	case RedisSlaveRole:
		return RedisSlaveRole
	default:
		if n.MasterReferent != "" {
			return RedisSlaveRole
		}
		if len(n.Slots) > 0 {
			return RedisMasterRole
		}
	}
	return "none"
}

// String string representation of a Instance
func (n *Node) String() string {
	if n.ServerStartTime.IsZero() {
		return fmt.Sprintf("{Redis ID: %s, role: %s, master: %s, link: %s, status: %s, addr: %s, slots: %s, len(migratingSlots): %d, len(importingSlots): %d}", n.ID, n.GetRole(), n.MasterReferent, n.LinkState, n.FailStatus, n.IPPort(), SlotSlice(n.Slots), len(n.MigratingSlots), len(n.ImportingSlots))
	}
	return fmt.Sprintf("{Redis ID: %s, role: %s, master: %s, link: %s, status: %s, addr: %s, slots: %s, len(migratingSlots): %d, len(importingSlots): %d, ServerStartTime: %s}", n.ID, n.GetRole(), n.MasterReferent, n.LinkState, n.FailStatus, n.IPPort(), SlotSlice(n.Slots), len(n.MigratingSlots), len(n.ImportingSlots), n.ServerStartTime.Format("2006-01-02 15:04:05"))
}

// IPPort returns join Ip Port string
func (n *Node) IPPort() string {
	return net.JoinHostPort(n.IP, n.Port)
}

// GetNodesByFunc returns first node found by the FindNodeFunc
func (n Nodes) GetNodesByFunc(f FindNodeFunc) Nodes {
	nodes := Nodes{}
	for _, node := range n {
		if f(node) {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// Clear used to clear possible ressources attach to the current Node
func (n *Node) Clear() {

}

// SetLinkStatus set the Node link status
func (n *Node) SetLinkStatus(status string) {
	n.LinkState = "" // reset value before setting the new one
	switch status {
	case RedisLinkStateConnected:
		n.LinkState = RedisLinkStateConnected
	case RedisLinkStateDisconnected:
		n.LinkState = RedisLinkStateDisconnected
	}
}

// SetFailureStatus set from inputs flags the possible failure status
func (n *Node) SetFailureStatus(flags string) {
	n.FailStatus = []string{} // reset value before setting the new one
	vals := strings.Split(flags, ",")
	for _, val := range vals {
		switch val {
		case NodeStatusFail:
			n.FailStatus = append(n.FailStatus, NodeStatusFail)
		case NodeStatusPFail:
			n.FailStatus = append(n.FailStatus, NodeStatusPFail)
		case NodeStatusHandshake:
			n.FailStatus = append(n.FailStatus, NodeStatusHandshake)
		case NodeStatusNoAddr:
			n.FailStatus = append(n.FailStatus, NodeStatusNoAddr)
		case NodeStatusNoFlags:
			n.FailStatus = append(n.FailStatus, NodeStatusNoFlags)
		}
	}
}

// SetReferentMaster set the redis node parent referent
func (n *Node) SetReferentMaster(ref string) {
	n.MasterReferent = ""
	if ref == "-" {
		return
	}
	n.MasterReferent = ref
}

// TotalSlots return the total number of slot
func (n *Node) TotalSlots() int {
	return len(n.Slots)
}

// HasStatus returns true if the node has the provided fail status flag
func (n *Node) HasStatus(flag string) bool {
	for _, status := range n.FailStatus {
		if status == flag {
			return true
		}
	}
	return false
}

// SortNodes sort Nodes and return the sorted Nodes
func (n Nodes) SortNodes() Nodes {
	sort.Sort(n)
	return n
}

// CountByFunc gives the number elements of NodeSlice that return true for the passed func.
func (n Nodes) CountByFunc(fn FindNodeFunc) (result int) {
	for _, v := range n {
		if fn(v) {
			result++
		}
	}
	return
}

// FilterByFunc remove a node from a slice by node ID and returns the slice. If not found, fail silently. Value must be unique
func (n Nodes) FilterByFunc(fn FindNodeFunc) Nodes {
	newSlice := Nodes{}
	for _, node := range n {
		if fn(node) {
			newSlice = append(newSlice, node)
		}
	}
	return newSlice
}

// SortByFunc returns a new ordered NodeSlice, determined by a func defining ‘less’.
func (n Nodes) SortByFunc(less func(*Node, *Node) bool) Nodes {
	result := make(Nodes, len(n))
	copy(result, n)
	by(less).Sort(n)
	return result
}

// Len is the number of elements in the collection.
func (n Nodes) Len() int {
	return len(n)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (n Nodes) Less(i, j int) bool {
	return n[i].ID < n[j].ID
}

// Swap swaps the elements with indexes i and j.
func (n Nodes) Swap(i, j int) {
	n[i], n[j] = n[j], n[i]
}

// By is the type of a "less" function that defines the ordering of its Node arguments.
type by func(p1, p2 *Node) bool

// Sort is a method on the function type, By, that sorts the argument slice according to the function.
func (b by) Sort(nodes Nodes) {
	ps := &nodeSorter{
		nodes: nodes,
		by:    b, // The Sort method's receiver is the function (closure) that defines the sort order.
	}
	sort.Sort(ps)
}

// nodeSorter joins a By function and a slice of Nodes to be sorted.
type nodeSorter struct {
	nodes Nodes
	by    func(p1, p2 *Node) bool // Closure used in the Less method.
}

// Len is part of sort.Interface.
func (s *nodeSorter) Len() int {
	return len(s.nodes)
}

// Swap is part of sort.Interface.
func (s *nodeSorter) Swap(i, j int) {
	s.nodes[i], s.nodes[j] = s.nodes[j], s.nodes[i]
}

// Less is part of sort.Interface. It is implemented by calling the "by" closure in the sorter.
func (s *nodeSorter) Less(i, j int) bool {
	return s.by(s.nodes[i], s.nodes[j])
}

// LessByID compare 2 Nodes with there ID
func LessByID(n1, n2 *Node) bool {
	return n1.ID < n2.ID
}

// MoreByID compare 2 Nodes with there ID
func MoreByID(n1, n2 *Node) bool {
	return n1.ID > n2.ID
}

// GetAllNodeIds function for get source master redis node ID to reshard slot
func (n Nodes) GetNodeIds(fn func(*Node) bool) []string {
	filterNodes := n.FilterByFunc(fn)
	var ids []string
	for _, n := range filterNodes {
		ids = append(ids, n.ID)
	}
	return ids
}

// GetNodeIdsWithOutIpPort function for get source master redis node ID to reshard slot
func (n Nodes) GetNodeWithNoIpPort(fn func(*Node) bool, ip, port string) Nodes {
	filterNodes := n.FilterByFunc(fn)
	for _, node := range n {
		if node.IP == ip && node.Port == port {
			continue
		}
		filterNodes = append(filterNodes, node)
	}
	return filterNodes
}

// GetNodeByID returns a Redis Node by its ID
func (n Nodes) GetNodeByID(id string) *Node {
	for _, node := range n {
		if node.ID == id {
			return node
		}
	}
	return nil
}

// GetNodeByIpPort
func (n Nodes) GetNodeByIpPort(ip, port string) *Node {
	for _, node := range n {
		if node.IP == ip && node.Port == port {
			return node
		}
	}
	return nil
}

// AllNodes anonymous function for fetch all Nodes
var AllNodes = func(n *Node) bool {
	return true
}

// IsMaster anonymous function for searching Master Node
var IsMaster = func(n *Node) bool {
	return n.Role == RedisMasterRole
}

// IsSlave anonymous function for searching Slave Node
var IsSlave = func(n *Node) bool {
	return n.Role == RedisSlaveRole
}

// IsMasterWithNoSlot anonymous function for searching Master Node with no slot
var IsMasterWithNoSlot = func(n *Node) bool {
	if (n.Role == RedisMasterRole) && (len(n.Slots) == 0) {
		return true
	}
	return false
}

// IsMasterWithSlot anonymous function for searching Master Node withslot
var IsMasterWithSlot = func(n *Node) bool {
	if (n.Role == RedisMasterRole) && (len(n.Slots) != 0) {
		return true
	}
	return false
}

func DecodeNodeInfos(input *string) Nodes {
	nodes := Nodes{}
	lines := strings.Split(*input, "\n")
	for _, line := range lines {
		values := strings.Split(line, " ")
		if len(values) < 8 {
			// last line is always empty
			klog.V(2).Infof("Not enough values in line split, ignoring line: %s", line)
			continue
		} else {
			node := NewDefaultNode()

			node.ID = values[0]
			//remove trailing port for cluster internal protocol
			ipPort := strings.Split(values[1], "@")
			if ip, port, err := net.SplitHostPort(ipPort[0]); err == nil {
				node.IP = ip
				node.Port = port
			} else {
				klog.Errorf("Error while decoding node info for node %s, cannot split ip:port (%s): %v", node.ID, values[1], err)
			}
			node.SetRole(values[2])
			node.SetFailureStatus(values[2])
			node.SetReferentMaster(values[3])
			if i, err := strconv.ParseInt(values[4], 10, 64); err == nil {
				node.PingSent = i
			}
			if i, err := strconv.ParseInt(values[5], 10, 64); err == nil {
				node.PongRecv = i
			}
			if i, err := strconv.ParseInt(values[6], 10, 64); err == nil {
				node.ConfigEpoch = i
			}
			node.SetLinkStatus(values[7])

			for _, slot := range values[8:] {
				if s, importing, migrating, err := DecodeSlotRange(slot); err == nil {
					node.Slots = append(node.Slots, s...)
					if importing != nil {
						node.ImportingSlots[importing.SlotID] = importing.FromNodeID
					}
					if migrating != nil {
						node.MigratingSlots[migrating.SlotID] = migrating.ToNodeID
					}
				}
			}
			nodes = append(nodes, node)
		}
	}
	return nodes
}
