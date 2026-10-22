package adminuser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestNullable(t *testing.T) {
	var in struct {
		A Nullable[uint64]    `json:"a"`
		B Nullable[uint64]    `json:"b"`
		C Nullable[uint64]    `json:"c"`
		T Nullable[time.Time] `json:"t"`
	}
	if err := json.Unmarshal([]byte(`{"a": 5, "b": null, "t": "2027-01-02T03:04:05Z"}`), &in); err != nil {
		t.Fatal(err)
	}
	if !in.A.Set || !in.A.Valid || in.A.Value != 5 || *in.A.Ptr() != 5 {
		t.Fatalf("a = %+v", in.A)
	}
	if !in.B.Set || in.B.Valid || in.B.Ptr() != nil {
		t.Fatalf("b = %+v", in.B)
	}
	if in.C.Set || !in.C.IsZero() {
		t.Fatalf("c = %+v", in.C)
	}
	if !in.T.Valid || !in.T.Value.Equal(time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("t = %+v", in.T)
	}
	if err := json.Unmarshal([]byte(`{"a": "x"}`), &in); err == nil {
		t.Fatal("type error not reported")
	}
	out, err := json.Marshal(struct {
		A Nullable[int] `json:"a,omitzero"`
		B Nullable[int] `json:"b,omitzero"`
		C Nullable[int] `json:"c,omitzero"`
	}{A: Of(1), B: Null[int]()})
	if err != nil || string(out) != `{"a":1,"b":null}` {
		t.Fatalf("marshal = %s, %v", out, err)
	}
}

func TestEscapeLikeAndCSV(t *testing.T) {
	if got := escapeLike("50%_off!"); got != "50!%!_off!!" {
		t.Fatalf("escapeLike = %q", got)
	}
	for in, want := range map[string]string{"=SUM(A1)": "'=SUM(A1)", "+1": "'+1", "@x": "'@x", "-": "'-", "a@example.com": "a@example.com", "": ""} {
		if got := csvSafe(in); got != want {
			t.Errorf("csvSafe(%q) = %q, want %q", in, got, want)
		}
	}
	s := randomString(64, lowerAlnum)
	if len(s) != 64 || strings.Trim(s, lowerAlnum) != "" {
		t.Fatalf("randomString = %q", s)
	}
}

func TestFiltersLikeIsLiteral(t *testing.T) {
	a := testutil.NewTestApp(t)
	testutil.CreateUser(t, a, "a_b@example.com")
	testutil.CreateUser(t, a, "axb@example.com")
	u := testutil.CreateUser(t, a, "c@example.com")
	if err := a.DB.Model(u).Update("remarks", "VIP 100% Customer").Error; err != nil {
		t.Fatal(err)
	}
	count := func(f Filters) int64 {
		var n int64
		if err := f.Apply(a.DB.Model(&model.User{}), time.Now()).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(Filters{Email: "A_B"}); n != 1 {
		t.Fatalf("email a_b matched %d", n)
	}
	if n := count(Filters{Remarks: "100%"}); n != 1 {
		t.Fatalf("remarks 100%% matched %d", n)
	}
	if n := count(Filters{Remarks: "vip 1000"}); n != 0 {
		t.Fatalf("remarks vip 1000 matched %d", n)
	}
}

func TestDeleteLastAdmin(t *testing.T) {
	a := testutil.NewTestApp(t)
	admin := testutil.CreateAdmin(t, a, "root@example.com")
	err := New(a).Delete(context.Background(), nil, admin.ID)
	var he *httpx.Error
	if !errors.As(err, &he) || he.Status != http.StatusConflict {
		t.Fatalf("delete last admin = %v", err)
	}
	other := testutil.CreateAdmin(t, a, "second@example.com")
	if err := New(a).Delete(context.Background(), nil, other.ID); err != nil {
		t.Fatalf("delete second admin: %v", err)
	}
}

func TestMailMoreThanQueueCapacity(t *testing.T) {
	a := testutil.NewTestApp(t)
	const n = 1300 // the default mail queue holds 1024 messages
	users := make([]model.User, n)
	for i := range users {
		users[i] = model.User{Email: fmt.Sprintf("bulk%04d@example.com", i), PasswordHash: "!", UUID: fmt.Sprintf("uuid-%d", i), Token: fmt.Sprintf("token-%d", i)}
	}
	if err := a.DB.CreateInBatches(&users, 500).Error; err != nil {
		t.Fatal(err)
	}
	queued, err := New(a).Mail(context.Background(), MailInput{Filters: Filters{Email: "bulk"}, Subject: "s", Content: "c"}, time.Now())
	if err != nil || queued != n {
		t.Fatalf("queued = %d, %v", queued, err)
	}
	box := testutil.Mailbox(t, a)
	deadline := time.Now().Add(20 * time.Second)
	for len(box.Messages()) < n && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := len(box.Messages()); got != n {
		t.Fatalf("delivered %d of %d", got, n)
	}
}
