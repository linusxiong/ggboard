package admin_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/linusxiong/ggboard/internal/service/adminuser"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Staff get uuid and token blanked, so they must not be able to search by them either: with ?token= a staff member
// could confirm a guessed or leaked subscription token and learn whose it is. The two filters are ignored for staff
// (the full list comes back) and still work for administrators.
func TestAdminUsersCredentialFiltersAreAdminOnly(t *testing.T) {
	e := newOpsEnv(t)
	target := e.reload(t, e.user.ID)
	for _, query := range []url.Values{{"token": {target.Token}}, {"uuid": {target.UUID}}} {
		path := "/api/v1/admin/users?" + query.Encode()
		views, total := testutil.DecodeList[adminuser.View](t, testutil.Do(t, e.r, http.MethodGet, path, nil, e.adminTok))
		if total != 1 || len(views) != 1 || views[0].ID != target.ID {
			t.Fatalf("admin %s: total=%d views=%+v", query.Encode(), total, views)
		}
		rec := testutil.Do(t, e.r, http.MethodGet, path, nil, e.staffTok)
		testutil.ExpectStatus(t, rec, http.StatusOK)
		views, total = testutil.DecodeList[adminuser.View](t, rec)
		if total != 3 || len(views) != 3 {
			t.Fatalf("staff %s narrowed the list: total=%d views=%d", query.Encode(), total, len(views))
		}
		for _, v := range views {
			if v.Token != "" || v.UUID != "" {
				t.Fatalf("staff view carries credentials: %+v", v)
			}
		}
		// A token that matches nobody: still the full list for staff (no "0 results" answer either).
		miss := url.Values{}
		for k := range query {
			miss.Set(k, "00000000000000000000000000000000")
		}
		if _, total := testutil.DecodeList[adminuser.View](t, testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/users?"+miss.Encode(), nil, e.staffTok)); total != 3 {
			t.Fatalf("staff %s with no match: total=%d", miss.Encode(), total)
		}
	}
}
