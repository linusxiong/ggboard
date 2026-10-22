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
	if counts[7] != 3 || counts[8] != 1 || s.OnlineUserCount() != 2 {
		t.Fatalf("AliveCounts = %v", counts)
	}
	if n := s.NodeAlive(2); len(n) != 2 || len(n[7]) != 2 {
		t.Fatalf("NodeAlive = %v", n)
	}

	s.SetNodeAlive(2, map[uint64][]string{8: {"9.9.9.9"}, 9: nil})
	if s.AliveCount(7) != 2 {
		t.Fatal("SetNodeAlive must replace the node's entries")
	}
	if st, _ := s.NodeStatus(2); st.OnlineUsers != 1 {
		t.Fatalf("online = %d", st.OnlineUsers)
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
	s.SetOnline(1, 5)
	st, _ = s.NodeStatus(1)
	if st.State(now, w) != StateOnline || st.Online(now, w) != 5 {
		t.Fatalf("pushing node = %+v", st)
	}
	later := now.Add(3 * time.Minute)
	if st.State(later, w) != StateOffline || st.Online(later, w) != 0 {
		t.Fatal("stale node must be offline with 0 online")
	}
	if len(s.Snapshot()) != 1 {
		t.Fatal("snapshot")
	}
	now = now.Add(time.Hour)
	s.Purge()
	if st, _ := s.NodeStatus(1); st.OnlineUsers != 0 {
		t.Fatal("purge must zero online users of stale nodes")
	}
	s.Forget(1)
	if len(s.Snapshot()) != 0 {
		t.Fatal("forget")
	}
}
