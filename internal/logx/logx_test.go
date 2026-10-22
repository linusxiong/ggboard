package logx

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]slog.Level{"debug": slog.LevelDebug, "": slog.LevelInfo, "WARN": slog.LevelWarn, "warning": slog.LevelWarn, "error": slog.LevelError} {
		got, err := ParseLevel(in)
		if err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseLevel("loud"); err == nil {
		t.Error("expected error")
	}
}

func TestRingCapturesRecords(t *testing.T) {
	ring := NewRing(3)
	var out bytes.Buffer
	log, err := New("info", "json", &out, ring)
	if err != nil {
		t.Fatal(err)
	}
	log.Debug("hidden")
	log.With("component", "mail").WithGroup("req").Info("first", "id", 7, "err", errors.New("boom"))
	log.Warn("second")
	log.Error("third")
	log.Info("fourth")

	if ring.Len() != 3 {
		t.Fatalf("len = %d", ring.Len())
	}
	all := ring.Entries(slog.LevelDebug, 0)
	if all[0].Msg != "fourth" || all[2].Msg != "second" {
		t.Fatalf("order = %v", []string{all[0].Msg, all[1].Msg, all[2].Msg})
	}
	if got := ring.Entries(slog.LevelWarn, 0); len(got) != 2 || got[0].Msg != "third" {
		t.Fatalf("filtered = %+v", got)
	}
	if got := ring.Entries(slog.LevelDebug, 1); len(got) != 1 {
		t.Fatalf("limit = %d", len(got))
	}
	if !strings.Contains(out.String(), `"msg":"fourth"`) || strings.Contains(out.String(), "hidden") {
		t.Fatalf("output = %s", out.String())
	}

	ring2 := NewRing(0)
	log2, _ := New("debug", "text", &bytes.Buffer{}, ring2)
	log2.With("component", "mail").WithGroup("req").Info("msg", "id", 7, "err", errors.New("boom"))
	e := ring2.Entries(slog.LevelDebug, 0)[0]
	if e.Attrs["component"] != "mail" || e.Attrs["req.id"] != int64(7) || e.Attrs["req.err"] != "boom" || e.Level != "info" {
		t.Fatalf("attrs = %#v level=%s", e.Attrs, e.Level)
	}
}

func TestNewRejectsUnknownFormat(t *testing.T) {
	if _, err := New("info", "xml", &bytes.Buffer{}, nil); err == nil {
		t.Fatal("expected error")
	}
}
