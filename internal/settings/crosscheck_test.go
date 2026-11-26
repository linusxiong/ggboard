package settings

import (
	"errors"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
)

// The three commission_distribution levels are shares of one order's commission: a save leaving them above 100% in
// total is refused (nothing written), checked against the stored levels the save does not touch. A stored violation
// does not block saves that leave the levels alone.
func TestCommissionLevelsSumAtMost100(t *testing.T) {
	d := newDB(t)
	s, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set(map[string]any{"commission_distribution_l1": 50, "commission_distribution_l2": 30, "commission_distribution_l3": 20}); err != nil {
		t.Fatalf("exactly 100%%: %v", err)
	}
	err = s.Set(map[string]any{"commission_distribution_l2": 31, "app_name": "Changed"})
	var fe FieldErrors
	if !errors.As(err, &fe) || len(fe["commission_distribution_l2"]) == 0 || len(fe["commission_distribution_l1"]) != 0 {
		t.Fatalf("101%% with one level changed: %v", err)
	}
	if s.Int("commission_distribution_l2") != 30 || s.String("app_name") == "Changed" {
		t.Fatal("a refused save wrote values")
	}
	err = s.Set(map[string]any{"commission_distribution_l1": 100, "commission_distribution_l3": 1})
	if !errors.As(err, &fe) || len(fe["commission_distribution_l1"]) == 0 || len(fe["commission_distribution_l3"]) == 0 {
		t.Fatalf("101%% with two levels changed: %v", err)
	}
	// Resetting a level to its default (0) counts as 0.
	if err := s.Set(map[string]any{"commission_distribution_l1": 80, "commission_distribution_l2": nil}); err != nil {
		t.Fatalf("80 + default 0 + 20: %v", err)
	}

	// A violation stored outside Set (an import) keeps unrelated saves working.
	if err := d.Save(&model.Setting{Key: "commission_distribution_l2", Value: "90"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	if s.Int("commission_distribution_l2") != 90 {
		t.Fatalf("stored l2 = %d", s.Int("commission_distribution_l2"))
	}
	if err := s.Set(map[string]any{"app_name": "Changed"}); err != nil {
		t.Fatalf("unrelated save with a stored violation: %v", err)
	}
}
