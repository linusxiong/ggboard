// Package logx builds the process slog.Logger and keeps the most recent records in an in-memory ring buffer
// (served by GET /admin/system/logs).
package logx

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
)

// DefaultRingSize is the number of records kept by NewRing(0).
const DefaultRingSize = 1000

// ParseLevel parses debug|info|warn|warning|error (case-insensitive).
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("logx: unknown level %q", s)
}

// New returns a logger writing text or JSON to w at the given level. When ring is non-nil every emitted
// record is also stored in it.
func New(level, format string, w io.Writer, ring *Ring) (*slog.Logger, error) {
	lvl, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler
	switch strings.ToLower(format) {
	case "json":
		h = slog.NewJSONHandler(w, opts)
	case "", "text":
		h = slog.NewTextHandler(w, opts)
	default:
		return nil, fmt.Errorf("logx: unknown format %q", format)
	}
	if ring != nil {
		h = &teeHandler{next: h, ring: ring}
	}
	return slog.New(h), nil
}

// Discard returns a logger that drops everything (useful in tests).
func Discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

// Entry is one buffered log record.
type Entry struct {
	Time  time.Time      `json:"time"`
	Level string         `json:"level"`
	Msg   string         `json:"msg"`
	Attrs map[string]any `json:"attrs,omitempty"`
	level slog.Level
}

// Ring is a fixed-size, concurrency-safe buffer of the latest log entries.
type Ring struct {
	mu    sync.Mutex
	buf   []Entry
	next  int
	count int
}

// NewRing returns a ring holding size entries (DefaultRingSize when size <= 0).
func NewRing(size int) *Ring {
	if size <= 0 {
		size = DefaultRingSize
	}
	return &Ring{buf: make([]Entry, size)}
}

func (r *Ring) add(e Entry) {
	r.mu.Lock()
	r.buf[r.next] = e
	r.next = (r.next + 1) % len(r.buf)
	if r.count < len(r.buf) {
		r.count++
	}
	r.mu.Unlock()
}

// Entries returns up to limit entries at or above min, newest first. limit <= 0 means all.
func (r *Ring) Entries(min slog.Level, limit int) []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Entry, 0, min2(r.count, limit))
	for i := 0; i < r.count; i++ {
		idx := (r.next - 1 - i + len(r.buf)) % len(r.buf)
		e := r.buf[idx]
		if e.level < min {
			continue
		}
		out = append(out, e)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// Len returns the number of buffered entries.
func (r *Ring) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

func min2(count, limit int) int {
	if limit > 0 && limit < count {
		return limit
	}
	return count
}

type teeHandler struct {
	next   slog.Handler
	ring   *Ring
	attrs  []slog.Attr // keys already prefixed with their group path
	prefix string
}

func (h *teeHandler) Enabled(ctx context.Context, l slog.Level) bool { return h.next.Enabled(ctx, l) }

func (h *teeHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := make(map[string]any, len(h.attrs)+r.NumAttrs())
	for _, a := range h.attrs {
		flatten(attrs, "", a)
	}
	r.Attrs(func(a slog.Attr) bool {
		flatten(attrs, h.prefix, a)
		return true
	})
	if len(attrs) == 0 {
		attrs = nil
	}
	h.ring.add(Entry{Time: r.Time, Level: strings.ToLower(r.Level.String()), Msg: r.Message, Attrs: attrs, level: r.Level})
	return h.next.Handle(ctx, r)
}

func (h *teeHandler) WithAttrs(as []slog.Attr) slog.Handler {
	nh := *h
	nh.next = h.next.WithAttrs(as)
	nh.attrs = slices.Clip(h.attrs)
	for _, a := range as {
		a.Key = h.prefix + a.Key
		nh.attrs = append(nh.attrs, a)
	}
	return &nh
}

func (h *teeHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	nh := *h
	nh.next = h.next.WithGroup(name)
	nh.prefix = h.prefix + name + "."
	return &nh
}

func flatten(dst map[string]any, prefix string, a slog.Attr) {
	v := a.Value.Resolve()
	if a.Key == "" && v.Kind() != slog.KindGroup {
		return
	}
	key := prefix + a.Key
	switch v.Kind() {
	case slog.KindGroup:
		p := prefix
		if a.Key != "" {
			p = key + "."
		}
		for _, ga := range v.Group() {
			flatten(dst, p, ga)
		}
	case slog.KindString:
		dst[key] = v.String()
	case slog.KindInt64:
		dst[key] = v.Int64()
	case slog.KindUint64:
		dst[key] = v.Uint64()
	case slog.KindFloat64:
		dst[key] = v.Float64()
	case slog.KindBool:
		dst[key] = v.Bool()
	case slog.KindDuration:
		dst[key] = v.Duration().String()
	case slog.KindTime:
		dst[key] = v.Time().Format(time.RFC3339Nano)
	default:
		if err, ok := v.Any().(error); ok {
			dst[key] = err.Error()
		} else {
			dst[key] = fmt.Sprint(v.Any())
		}
	}
}
