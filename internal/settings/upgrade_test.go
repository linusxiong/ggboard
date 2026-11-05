package settings

import (
	"slices"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
)

// A site that saved telegram_events before ticket_user_reply existed gets the event once, since a saved list replaces
// the default. A list with no events is left alone, and turning the event off later sticks.
func TestAddTicketUserReplyEvent(t *testing.T) {
	cases := []struct{ name, saved, want string }{
		{"saved list", `["order_paid","ticket_new"]`, `["order_paid","ticket_new","ticket_user_reply"]`},
		{"already there", `["ticket_user_reply"]`, `["ticket_user_reply"]`},
		{"no alerts chosen", `[]`, `[]`},
		{"corrupt row", `not json`, `not json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newDB(t)
			if err := db.Create(&model.Setting{Key: "telegram_events", Value: tc.saved}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := New(db); err != nil {
				t.Fatal(err)
			}
			var row model.Setting
			if err := db.Where(&model.Setting{Key: "telegram_events"}).First(&row).Error; err != nil {
				t.Fatal(err)
			}
			if row.Value != tc.want {
				t.Fatalf("telegram_events = %q, want %q", row.Value, tc.want)
			}

			row.Value = `["order_paid"]`
			if err := db.Save(&row).Error; err != nil {
				t.Fatal(err)
			}
			s, err := New(db)
			if err != nil {
				t.Fatal(err)
			}
			if got := s.Strings("telegram_events"); !slices.Equal(got, []string{"order_paid"}) {
				t.Fatalf("second start changed telegram_events to %v", got)
			}
		})
	}
}

// A new site uses the default, which has the event; the administrator's later choice is not overridden.
func TestTicketUserReplyEventOnNewSite(t *testing.T) {
	db := newDB(t)
	s, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(s.Strings("telegram_events"), ticketUserReplyEvent) {
		t.Fatalf("default = %v", s.Strings("telegram_events"))
	}
	if err := s.Set(map[string]any{"telegram_events": []string{"order_paid"}}); err != nil {
		t.Fatal(err)
	}
	if s, err = New(db); err != nil {
		t.Fatal(err)
	}
	if got := s.Strings("telegram_events"); !slices.Equal(got, []string{"order_paid"}) {
		t.Fatalf("restart changed telegram_events to %v", got)
	}
}
