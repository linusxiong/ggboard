package subscription_test

import (
	"context"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/app/online"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestAvailableNodes(t *testing.T) {
	a := testutil.NewTestApp(t)
	ctx := context.Background()
	mk := func(name string, sort int, show bool, groups []uint64, parent *uint64, ps model.JSONMap) *model.Node {
		n := &model.Node{Type: model.NodeVmess, Name: name, Host: name + ".example.com", Port: "443", ServerPort: 443,
			GroupIDs: groups, Rate: 1, Show: show, Sort: sort, ParentID: parent, ProtocolSettings: ps}
		if err := a.DB.Create(n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	hiddenParent := mk("parent", 9, false, []uint64{2}, nil, model.JSONMap{"network": "ws"})
	b := mk("b", 2, true, []uint64{1, 2}, nil, nil)
	a1 := mk("a", 1, true, []uint64{1}, nil, nil)
	mk("hidden", 0, false, []uint64{1}, nil, nil)
	mk("other-group", 0, true, []uint64{3}, nil, nil)
	child := mk("child", 3, true, []uint64{1}, &hiddenParent.ID, nil)
	missing := uint64(999)
	mk("orphan", 4, true, []uint64{1}, &missing, nil)
	mk("broken", 5, true, []uint64{1}, nil, model.JSONMap{"tls": "not-a-number"})
	same := mk("c", 2, true, []uint64{1}, nil, nil) // same sort as b: ordered by id

	a.Online.TouchPush(hiddenParent.ID)
	a.Online.Touch(b.ID)

	gid := uint64(1)
	u := &model.User{ID: 1, UUID: "3b1b6f5e-7c2a-4d8e-9f10-2a3b4c5d6e7f", GroupID: &gid}
	nodes, err := subscription.AvailableNodes(ctx, a, u)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, n := range nodes {
		got = append(got, n.Node.Name)
	}
	if want := "a,b,c,child"; strings.Join(got, ",") != want {
		t.Fatalf("nodes = %s, want %s", strings.Join(got, ","), want)
	}
	if nodes[0].Node.ID != a1.ID || nodes[2].Node.ID != same.ID {
		t.Fatal("order")
	}
	if nodes[3].Effective.ID != hiddenParent.ID || nodes[3].Status != online.StateOnline || !nodes[3].IsOnline {
		t.Fatalf("child = %+v", nodes[3])
	}
	if nodes[1].Status != online.StateIdle || !nodes[1].IsOnline || nodes[0].Status != online.StateOffline || nodes[0].IsOnline {
		t.Fatal("three-state status")
	}
	ps := subscription.NodeProxies(u, nodes, subscription.ProxyOptions{ShowProtocol: true, Keywords: []string{"child"}})
	if len(ps) != 1 || ps[0].Name != "[vmess] child" || ps[0].Network != "ws" || ps[0].Host != child.Host || ps[0].Status != online.StateOnline {
		t.Fatalf("child proxy = %+v", ps)
	}

	// Cached for NodesTTL: a new node shows up only after invalidation.
	mk("d", 0, true, []uint64{1}, nil, nil)
	if again, _ := subscription.AvailableNodes(ctx, a, u); len(again) != 4 {
		t.Fatalf("cached list = %d", len(again))
	}
	subscription.InvalidateNodes(a)
	if again, _ := subscription.AvailableNodes(ctx, a, u); len(again) != 5 || again[0].Node.Name != "d" {
		t.Fatalf("after invalidation = %d", len(again))
	}

	// Users without a group see nothing.
	if none, err := subscription.AvailableNodes(ctx, a, &model.User{ID: 2}); err != nil || none != nil {
		t.Fatal("no group")
	}
}
