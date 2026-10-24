package online

import (
	"testing"
	"time"
)

func TestAliveIPsAcrossNodes(t *testing.T) {
	now := time.Unix(10_000, 0)
	s := New(func() time.Duration { return time.Minute })
	s.SetClock(func() time.Time { return now })

	s.SetAliveIPs(1, 7, []string{"1.1.1.1", "2.2.2.2", "1.1.1.1"})
	s.SetAliveIPs(2, 7, []string{"2.2.2.2", "3.3.3.3"})
	s.SetAliveIPs(2, 8, []string{"9.9.9.9"})
	if got := s.AliveCount(7); got != 3 {
		t.Fatalf("AliveCount = %d", got)
	}
	if ips := s.AliveIPs(7); len(ips) != 3 || ips[0] != "1.1.1.1" {
		t.Fatalf("AliveIPs = %v", ips)
	}
	counts := s.AliveCounts()
	if counts[7] != 3 || counts[8] != 1 || s.OnlineUserCount(time.Minute) != 2 {
		t.Fatalf("AliveCounts = %v", counts)
	}
	if n := s.NodeOnline(2, time.Minute); len(n) != 2 || len(n[7]) != 2 {
		t.Fatalf("NodeOnline = %v", n)
	}

	s.SetNodeAlive(2, map[uint64][]string{8: {"9.9.9.9"}, 9: nil})
	if s.AliveCount(7) != 2 {
		t.Fatal("SetNodeAlive must replace the node's entries")
	}
	if got := s.NodeOnlineCount(2, time.Minute); got != 1 {
		t.Fatalf("online = %d", got)
	}

	s.SetAliveIPs(1, 7, nil)
	if s.AliveCount(7) != 0 {
		t.Fatal("empty list must remove entry")
	}

	s.SetAliveIPs(3, 10, []string{"4.4.4.4"})
	now = now.Add(2 * time.Minute)
	if s.AliveCount(10) != 0 {
		t.Fatal("expired IPs must be ignored")
	}
	if removed := s.Purge(); removed != 2 {
		t.Fatalf("purged %d", removed)
	}
}

func TestRelaxedCounting(t *testing.T) {
	s := New(nil)
	s.SetAliveIPs(1, 1, []string{"10.0.0.1", "10.0.0.2", "10.0.1.1", "2001:db8::1", "2001:db8::2", "::ffff:10.0.0.3"})
	if got := s.AliveCountMode(1, 0); got != 6 {
		t.Fatalf("strict = %d", got)
	}
	if got := s.AliveCountMode(1, 1); got != 3 {
		t.Fatalf("relaxed = %d", got)
	}
	if got := s.AliveCountsMode(1)[1]; got != 3 {
		t.Fatalf("relaxed counts = %d", got)
	}
	if CountIPs([]string{"a_1", "a_1", "b_2"}, true) != 2 {
		t.Fatal("non-IP entries are counted verbatim")
	}
}

func TestNodeStatusStates(t *testing.T) {
	now := time.Unix(50_000, 0)
	s := New(nil)
	s.SetClock(func() time.Time { return now })
	w := Window(60*time.Second, 30*time.Second)
	if w != 2*time.Minute {
		t.Fatalf("window = %v", w)
	}
	if _, ok := s.NodeStatus(1); ok {
		t.Fatal("unknown node")
	}
	s.Touch(1)
	s.SetVersion(1, "v2bx 1.0")
	s.SetLoad(1, Load{CPU: 12.5, Mem: 40})
	st, _ := s.NodeStatus(1)
	if st.State(now, w) != StateIdle || !st.IsOnline(now, w) || st.Version != "v2bx 1.0" || st.Load.CPU != 12.5 {
		t.Fatalf("status = %+v", st)
	}
	st.Load.CPU = 99 // copies must not alias the store
	if again, _ := s.NodeStatus(1); again.Load.CPU != 12.5 {
		t.Fatal("NodeStatus must return a copy")
	}
	s.TouchPush(1)
	s.SetTrafficUsers(1, []uint64{1, 2, 3, 4, 5, 5})
	st, _ = s.NodeStatus(1)
	if st.State(now, w) != StateOnline || s.NodeOnlineCount(1, w) != 5 {
		t.Fatalf("pushing node = %+v, online %d", st, s.NodeOnlineCount(1, w))
	}
	now = now.Add(3 * time.Minute)
	if st.State(now, w) != StateOffline || s.NodeOnlineCount(1, w) != 0 {
		t.Fatal("stale node must be offline with 0 online")
	}
	if len(s.Snapshot()) != 1 {
		t.Fatal("snapshot")
	}
	now = now.Add(time.Hour)
	s.Purge()
	if s.nodes[1].traffic != nil {
		t.Fatal("purge must drop the traffic users of stale nodes")
	}
	s.Forget(1)
	if len(s.Snapshot()) != 0 {
		t.Fatal("forget")
	}
}

// The node's online count and its online-users list are one set: the latest traffic report's users (while the
// report is within W) plus the users the node reported alive within W. Alive IPs older than W but still within the
// device TTL only fill in the IPs of users who are online through traffic. (Before, the count was the latest
// traffic report and the list every unexpired alive entry, so a node showed "1 online" over a list of 7.)
func TestNodeOnline(t *testing.T) {
	now := time.Unix(90_000, 0)
	s := New(func() time.Duration { return 5 * time.Minute })
	s.SetClock(func() time.Time { return now })
	w := 2 * time.Minute

	now = now.Add(-3 * time.Minute)
	s.SetAliveIPs(1, 4, []string{"192.0.2.4"}) // reported 3 min ago: unexpired, but outside W
	now = now.Add(3 * time.Minute)
	s.SetAliveIPs(1, 2, []string{"192.0.2.2"})
	s.SetAliveIPs(1, 3, []string{"192.0.2.3"})
	s.TouchPush(1)
	s.SetTrafficUsers(1, []uint64{1, 2})

	got := s.NodeOnline(1, w)
	if len(got) != 3 || got[1] == nil || len(got[1]) != 0 || got[2][0] != "192.0.2.2" || got[3][0] != "192.0.2.3" {
		t.Fatalf("NodeOnline = %#v", got)
	}
	if _, ok := got[4]; ok || s.NodeOnlineCount(1, w) != len(got) {
		t.Fatalf("user 4 is outside W; count %d", s.NodeOnlineCount(1, w))
	}
	got[2][0] = "changed" // copies must not alias the store
	if s.NodeOnline(1, w)[2][0] != "192.0.2.2" {
		t.Fatal("NodeOnline must return copies")
	}

	s.SetTrafficUsers(1, []uint64{4}) // traffic brings user 4 back, with its unexpired IPs
	if got = s.NodeOnline(1, w); len(got) != 3 || got[4][0] != "192.0.2.4" {
		t.Fatalf("after traffic from user 4: %#v", got)
	}
	s.TouchPush(2)
	s.SetTrafficUsers(2, []uint64{3, 9})
	if n := s.OnlineUserCount(w); n != 4 { // users 2, 3, 4, 9
		t.Fatalf("OnlineUserCount = %d", n)
	}

	now = now.Add(w + time.Second) // traffic reports and alive reports are now older than W
	if n := s.NodeOnlineCount(1, w); n != 0 || s.OnlineUserCount(w) != 0 || s.NodeOnlineCount(99, w) != 0 {
		t.Fatalf("stale node online = %d", n)
	}
	if len(s.NodeOnline(99, w)) != 0 {
		t.Fatal("unknown node")
	}
}
