// Package online keeps runtime node state (last check/push, online users, load) and the alive IPs reported
// by nodes. Everything is in memory; entries expire by TTL and are purged by the online_cleanup job.
//
// A user is online on a node within window W when the node's latest traffic report (at most W old) contains the
// user, or when the node reported the user's alive IPs at most W ago. The node's online count, its online-users
// list and the online-user totals all use this one definition (docs/NODE_PROTOCOLS.md §5.1).
package online

import (
	"net/netip"
	"sort"
	"sync"
	"time"
)

// Load is the node machine load reported by the node (percentages 0-100, uptime in seconds).
type Load struct {
	CPU    float64 `json:"cpu"`
	Mem    float64 `json:"mem"`
	Swap   float64 `json:"swap"`
	Disk   float64 `json:"disk"`
	Uptime int64   `json:"uptime"`
}

// Status is the runtime status of one node.
type Status struct {
	LastCheckAt *time.Time `json:"last_check_at"` // last config/user pull
	LastPushAt  *time.Time `json:"last_push_at"`  // last traffic report
	Load        *Load      `json:"load"`
	Version     string     `json:"version,omitempty"`
}

// LastSeen returns the most recent of LastCheckAt and LastPushAt (zero if never seen).
func (s Status) LastSeen() time.Time {
	var t time.Time
	if s.LastCheckAt != nil {
		t = *s.LastCheckAt
	}
	if s.LastPushAt != nil && s.LastPushAt.After(t) {
		t = *s.LastPushAt
	}
	return t
}

// Node states (docs/NODE_PROTOCOLS.md §5.1).
const (
	StateOffline = 0 // nothing within the window
	StateIdle    = 1 // online, no push: only LastCheckAt within the window
	StateOnline  = 2 // LastPushAt within the window
)

// Window returns W = 2 x max(pushInterval, pullInterval).
func Window(pushInterval, pullInterval time.Duration) time.Duration {
	return 2 * max(pushInterval, pullInterval)
}

// State returns StateOnline, StateIdle or StateOffline for window w.
func (s Status) State(now time.Time, w time.Duration) int {
	switch {
	case s.LastPushAt != nil && now.Sub(*s.LastPushAt) <= w:
		return StateOnline
	case s.LastCheckAt != nil && now.Sub(*s.LastCheckAt) <= w:
		return StateIdle
	}
	return StateOffline
}

// IsOnline reports whether the node was seen within maxAge of now.
func (s Status) IsOnline(now time.Time, maxAge time.Duration) bool {
	t := s.LastSeen()
	return !t.IsZero() && now.Sub(t) <= maxAge
}

type aliveEntry struct {
	ips     []string
	seen    time.Time // when the node last reported these IPs
	expires time.Time
}

type nodeState struct {
	status  Status
	alive   map[uint64]aliveEntry // userID -> IPs
	traffic map[uint64]struct{}   // users in the latest traffic report (at status.LastPushAt)
}

// online returns the users online within w (see the package doc), each with the node's live IPs (shared with the
// store, nil when the node reported none). The caller holds the lock.
func (n *nodeState) online(now time.Time, w time.Duration) map[uint64][]string {
	out := map[uint64][]string{}
	if n.status.LastPushAt != nil && now.Sub(*n.status.LastPushAt) <= w {
		for uid := range n.traffic {
			out[uid] = nil
		}
	}
	for uid, e := range n.alive {
		if !now.Before(e.expires) {
			continue
		}
		if _, ok := out[uid]; ok || now.Sub(e.seen) <= w {
			out[uid] = e.ips
		}
	}
	return out
}

func (n *nodeState) clone() Status {
	s := n.status
	if s.Load != nil {
		l := *s.Load
		s.Load = &l
	}
	if s.LastCheckAt != nil {
		t := *s.LastCheckAt
		s.LastCheckAt = &t
	}
	if s.LastPushAt != nil {
		t := *s.LastPushAt
		s.LastPushAt = &t
	}
	return s
}

// Store is safe for concurrent use.
type Store struct {
	mu    sync.RWMutex
	nodes map[uint64]*nodeState
	ttl   func() time.Duration
	now   func() time.Time
}

// DefaultTTL is used when the TTL function is nil or returns <= 0 (also the minimum used by app: the alive
// TTL is max(300s, 2 x server_push_interval)).
const DefaultTTL = 300 * time.Second

// New returns a store whose alive-IP entries live for aliveTTL() (evaluated on each write, so it can follow
// settings such as 2 x server_push_interval).
func New(aliveTTL func() time.Duration) *Store {
	return &Store{nodes: map[uint64]*nodeState{}, ttl: aliveTTL, now: time.Now}
}

// SetClock replaces the time source (tests).
func (s *Store) SetClock(now func() time.Time) {
	s.mu.Lock()
	s.now = now
	s.mu.Unlock()
}

func (s *Store) aliveTTL() time.Duration {
	if s.ttl != nil {
		if d := s.ttl(); d > 0 {
			return d
		}
	}
	return DefaultTTL
}

func (s *Store) node(id uint64) *nodeState {
	n := s.nodes[id]
	if n == nil {
		n = &nodeState{alive: map[uint64]aliveEntry{}}
		s.nodes[id] = n
	}
	return n
}

// Touch records a pull (config/users) from the node.
func (s *Store) Touch(nodeID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.now()
	s.node(nodeID).status.LastCheckAt = &t
}

// TouchPush records a push (traffic/alive/status) from the node.
func (s *Store) TouchPush(nodeID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.now()
	s.node(nodeID).status.LastPushAt = &t
}

// SetTrafficUsers replaces the users of the node's latest traffic report.
func (s *Store) SetTrafficUsers(nodeID uint64, userIDs []uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	set := make(map[uint64]struct{}, len(userIDs))
	for _, uid := range userIDs {
		set[uid] = struct{}{}
	}
	s.node(nodeID).traffic = set
}

// SetLoad records the node's machine load.
func (s *Store) SetLoad(nodeID uint64, load Load) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.node(nodeID).status.Load = &load
}

// SetVersion records the node backend version string.
func (s *Store) SetVersion(nodeID uint64, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.node(nodeID).status.Version = version
}

// SetAliveIPs replaces the alive IPs of userID on nodeID (an empty list removes the entry).
func (s *Store) SetAliveIPs(nodeID, userID uint64, ips []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.node(nodeID)
	if len(ips) == 0 {
		delete(n.alive, userID)
		return
	}
	now := s.now()
	n.alive[userID] = aliveEntry{ips: dedupe(ips), seen: now, expires: now.Add(s.aliveTTL())}
}

// SetNodeAlive replaces every alive entry of nodeID with alive (userID -> IPs).
func (s *Store) SetNodeAlive(nodeID uint64, alive map[uint64][]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.node(nodeID)
	now := s.now()
	exp := now.Add(s.aliveTTL())
	n.alive = make(map[uint64]aliveEntry, len(alive))
	for uid, ips := range alive {
		if len(ips) > 0 {
			n.alive[uid] = aliveEntry{ips: dedupe(ips), seen: now, expires: exp}
		}
	}
}

// AliveIPs returns the distinct live IPs of userID across all nodes, sorted.
func (s *Store) AliveIPs(userID uint64) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := s.now()
	set := map[string]struct{}{}
	for _, n := range s.nodes {
		if e, ok := n.alive[userID]; ok && now.Before(e.expires) {
			for _, ip := range e.ips {
				set[ip] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(set))
	for ip := range set {
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}

// AliveCount returns the number of distinct live IPs of userID across all nodes (device_limit_mode 0, strict).
func (s *Store) AliveCount(userID uint64) int { return len(s.AliveIPs(userID)) }

// AliveCountMode is AliveCount honoring device_limit_mode: 1 (relaxed) collapses IPv4 to /24 and IPv6 to /64.
func (s *Store) AliveCountMode(userID uint64, mode int) int {
	return CountIPs(s.AliveIPs(userID), mode == 1)
}

// AliveCountsMode is AliveCounts honoring device_limit_mode (see AliveCountMode).
func (s *Store) AliveCountsMode(mode int) map[uint64]int {
	if mode != 1 {
		return s.AliveCounts()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := s.now()
	sets := map[uint64][]string{}
	for _, n := range s.nodes {
		for uid, e := range n.alive {
			if now.Before(e.expires) {
				sets[uid] = append(sets[uid], e.ips...)
			}
		}
	}
	out := make(map[uint64]int, len(sets))
	for uid, ips := range sets {
		out[uid] = CountIPs(ips, true)
	}
	return out
}

// CountIPs counts distinct IPs; when relaxed, IPv4 addresses are collapsed to /24 and IPv6 to /64.
// Entries that are not IP addresses (e.g. "ip_node" strings) are counted verbatim.
func CountIPs(ips []string, relaxed bool) int {
	set := make(map[string]struct{}, len(ips))
	for _, raw := range ips {
		key := raw
		if relaxed {
			if addr, err := netip.ParseAddr(raw); err == nil {
				bits := 64
				if addr.Unmap().Is4() {
					addr, bits = addr.Unmap(), 24
				}
				if p, err := addr.Prefix(bits); err == nil {
					key = p.String()
				}
			}
		}
		set[key] = struct{}{}
	}
	return len(set)
}

// AliveCounts returns userID -> distinct live IP count for every user with live IPs (UniProxy alivelist).
func (s *Store) AliveCounts() map[uint64]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := s.now()
	sets := map[uint64]map[string]struct{}{}
	for _, n := range s.nodes {
		for uid, e := range n.alive {
			if !now.Before(e.expires) {
				continue
			}
			if sets[uid] == nil {
				sets[uid] = map[string]struct{}{}
			}
			for _, ip := range e.ips {
				sets[uid][ip] = struct{}{}
			}
		}
	}
	out := make(map[uint64]int, len(sets))
	for uid, set := range sets {
		out[uid] = len(set)
	}
	return out
}

// OnlineUserCount returns the number of distinct users online (NodeOnline) on any node within w.
func (s *Store) OnlineUserCount(w time.Duration) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := s.now()
	users := map[uint64]struct{}{}
	for _, n := range s.nodes {
		for uid := range n.online(now, w) {
			users[uid] = struct{}{}
		}
	}
	return len(users)
}

// NodeOnline returns the users online on nodeID within w (see the package doc), each with its live IPs on the
// node; the list is empty, never nil, when the node reported none (XrayR, or a user below the backend's
// minimum-traffic threshold).
func (s *Store) NodeOnline(nodeID uint64, w time.Duration) map[uint64][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[uint64][]string{}
	n := s.nodes[nodeID]
	if n == nil {
		return out
	}
	for uid, ips := range n.online(s.now(), w) {
		out[uid] = append(make([]string, 0, len(ips)), ips...)
	}
	return out
}

// NodeOnlineCount returns len(NodeOnline(nodeID, w)).
func (s *Store) NodeOnlineCount(nodeID uint64, w time.Duration) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := s.nodes[nodeID]
	if n == nil {
		return 0
	}
	return len(n.online(s.now(), w))
}

// NodeStatus returns a copy of the node's status.
func (s *Store) NodeStatus(nodeID uint64) (Status, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := s.nodes[nodeID]
	if n == nil {
		return Status{}, false
	}
	return n.clone(), true
}

// Snapshot returns a copy of every node status.
func (s *Store) Snapshot() map[uint64]Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[uint64]Status, len(s.nodes))
	for id, n := range s.nodes {
		out[id] = n.clone()
	}
	return out
}

// Forget drops all state of a node (e.g. after deletion).
func (s *Store) Forget(nodeID uint64) {
	s.mu.Lock()
	delete(s.nodes, nodeID)
	s.mu.Unlock()
}

// Purge removes expired alive entries, drops the traffic-report users of nodes that have not pushed within the
// TTL and returns the number of removed alive entries.
func (s *Store) Purge() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now, ttl, removed := s.now(), s.aliveTTL(), 0
	for _, n := range s.nodes {
		for uid, e := range n.alive {
			if !now.Before(e.expires) {
				delete(n.alive, uid)
				removed++
			}
		}
		if n.status.LastPushAt == nil || now.Sub(*n.status.LastPushAt) > ttl {
			n.traffic = nil
		}
	}
	return removed
}

func dedupe(ips []string) []string {
	seen := make(map[string]struct{}, len(ips))
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		if ip == "" {
			continue
		}
		if _, ok := seen[ip]; !ok {
			seen[ip] = struct{}{}
			out = append(out, ip)
		}
	}
	return out
}
