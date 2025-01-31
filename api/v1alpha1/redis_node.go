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
package v1alpha1

import (
	"net"
	"strings"
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

// IPPort returns join Ip Port string
func (n *Node) IPPort() string {
	return net.JoinHostPort(n.IP, n.Port)
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

// GetMasterNodes
func (n *Nodes) GetMasterNodes() *Nodes {
	newSlice := Nodes{}
	for _, node := range *n {
		if node.Role == RedisMasterRole {
			newSlice = append(newSlice, node)
		}
	}
	return &newSlice
}

// GetMasterNodesWithSlot
func (n *Nodes) GetMasterNodesWithSlot() *Nodes {
	newSlice := Nodes{}
	for _, node := range *n {
		if (node.Role == RedisMasterRole) && (len(node.Slots) != 0) {
			newSlice = append(newSlice, node)
		}
	}
	return &newSlice
}

// GetMasterNodesWithNoSlot
func (n *Nodes) GetMasterNodesWithNoSlot() *Nodes {
	newSlice := Nodes{}
	for _, node := range *n {
		if (node.Role == RedisMasterRole) && (len(node.Slots) == 0) {
			newSlice = append(newSlice, node)
		}
	}
	return &newSlice
}

// GetSlaveNodes
func (n *Nodes) GetSlaveNodes() *Nodes {
	newSlice := Nodes{}
	for _, node := range *n {
		if node.Role == RedisSlaveRole {
			newSlice = append(newSlice, node)
		}
	}
	return &newSlice
}

// GetNodeWithIPPort
func (n *Nodes) GetNodeWithIPPort(ip, port string) *Node {
	for _, node := range *n {
		if node.IP == ip && node.Port == port {
			return &node
		}
	}
	return nil
}

// GetNodeWithNoIPPort function for get source master redis node ID to reshard slot
func (n *Nodes) GetNodeWithNoIPPort(ip, port string) *Nodes {
	newSlice := Nodes{}
	for _, node := range *n {
		if node.IP == ip && node.Port == port {
			continue
		}
		newSlice = append(newSlice, node)
	}
	return &newSlice
}

// GetAllNodeIds function for get source master redis node ID to reshard slot
func (n *Nodes) GetNodeIds() []string {
	var ids []string
	for _, n := range *n {
		ids = append(ids, n.ID)
	}
	return ids
}

// GetNodeByID returns a Redis Node by its ID
func (n *Nodes) GetNodeByID(id string) *Node {
	for _, node := range *n {
		if node.ID == id {
			return &node
		}
	}
	return nil
}

// GetNodeByName returns a Redis Node by its ID
func (n *Nodes) GetNodeByName(name string) *Node {
	for _, node := range *n {
		if node.Name == name {
			return &node
		}
	}
	return nil
}

// GetNodeWithNoName returns a Redis Node by its ID
func (n *Nodes) GetNodeWithNoName(name string) *Nodes {
	newSlice := Nodes{}
	for _, node := range *n {
		if node.Name == name {
			continue
		}
		newSlice = append(newSlice, node)
	}
	return &newSlice
}

// CountNodes gives the number elements of NodeSlice
func (n Nodes) CountNodes() int {
	return len(n)
}

func (n *Nodes) GetIPPortByName(name string) string {
	for _, node := range *n {
		if node.Name == name {
			return node.IPPort()
		}
	}
	return ""
}

func (n *Nodes) GetIPPortsByRole(role string) []string {
	ipPorts := []string{}
	for _, node := range *n {
		if node.Role == role {
			ipPorts = append(ipPorts, node.IPPort())
		}
	}
	return ipPorts
}
