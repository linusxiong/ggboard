package user_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/api/user"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestUserNodesVisibility(t *testing.T) {
	a := testutil.NewTestApp(t)
	r := testutil.NewRouterWith(t, a, user.Register)
	ga, gb := uint64(1), uint64(2)
	for _, g := range []model.ServerGroup{{ID: ga, Name: "a"}, {ID: gb, Name: "b"}} {
		if err := a.DB.Create(&g).Error; err != nil {
			t.Fatal(err)
		}
	}
	mk := func(n model.Node) model.Node {
		t.Helper()
		n.Port, n.ServerPort, n.Rate = "443", 443, 1.5
		if err := a.DB.Omit(clause.Associations).Create(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	visible := mk(model.Node{Type: "vless", Name: "HK", Host: "hk.example.com", Show: true, Sort: 2, GroupIDs: model.JSONSlice[uint64]{ga, gb}, Tags: model.JSONSlice[string]{"HK"}})
	mk(model.Node{Type: "vless", Name: "hidden", Host: "h.example.com", Show: false, GroupIDs: model.JSONSlice[uint64]{ga}})
	mk(model.Node{Type: "trojan", Name: "other group", Host: "o.example.com", Show: true, GroupIDs: model.JSONSlice[uint64]{gb}})
	child := mk(model.Node{Type: "vless", Name: "relay", Host: "relay.example.com", Show: true, Sort: 1, GroupIDs: model.JSONSlice[uint64]{ga}, ParentID: &visible.ID})

	u := testutil.CreateUser(t, a, "alice@example.com", usersvc.CreateOptions{GroupID: &ga, TransferEnable: 1 << 30})
	tok := testutil.Login(t, a, u)
	type view struct {
		ID       uint64   `json:"id"`
		Name     string   `json:"name"`
		Type     string   `json:"type"`
		Tags     []string `json:"tags"`
		Rate     float64  `json:"rate"`
		Host     *string  `json:"host"`
		Port     *string  `json:"port"`
		IsOnline bool     `json:"is_online"`
		Sort     int      `json:"sort"`
	}
	got := testutil.Decode[[]view](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/nodes", nil, tok))
	if len(got) != 2 || got[0].ID != child.ID || got[1].ID != visible.ID || got[1].IsOnline || got[1].Host == nil || *got[1].Host != "hk.example.com" ||
		*got[1].Port != "443" || got[1].Rate != 1.5 || len(got[1].Tags) != 1 || got[0].Tags == nil {
		t.Fatalf("nodes %+v", got)
	}

	// A child is online when its parent is.
	a.Online.Touch(visible.ID)
	got = testutil.Decode[[]view](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/nodes", nil, tok))
	if !got[0].IsOnline || !got[1].IsOnline {
		t.Fatalf("online flags %+v", got)
	}

	if err := a.Settings.Set(map[string]any{"show_info_to_user": false}); err != nil {
		t.Fatal(err)
	}
	rec := testutil.Do(t, r, http.MethodGet, "/api/v1/user/nodes", nil, tok)
	var raw struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, n := range raw.Data {
		if _, ok := n["host"]; ok {
			t.Fatalf("host leaked: %s", rec.Body.String())
		}
		if _, ok := n["port"]; ok {
			t.Fatalf("port leaked: %s", rec.Body.String())
		}
	}

	noGroup := testutil.CreateUser(t, a, "bob@example.com", usersvc.CreateOptions{TransferEnable: 1 << 30})
	if got := testutil.Decode[[]view](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/nodes", nil, testutil.Login(t, a, noGroup))); len(got) != 0 {
		t.Fatalf("no group: %+v", got)
	}
	past := time.Now().Add(-time.Hour)
	expired := testutil.CreateUser(t, a, "carol@example.com", usersvc.CreateOptions{GroupID: &ga, TransferEnable: 1 << 30, ExpiredAt: &past})
	rec = testutil.Do(t, r, http.MethodGet, "/api/v1/user/nodes", nil, testutil.Login(t, a, expired))
	if testutil.ExpectStatus(t, rec, http.StatusOK); rec.Body.String() != `{"data":[]}` {
		t.Fatalf("expired user: %s", rec.Body.String())
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/nodes", nil, ""), http.StatusUnauthorized)
}
