package logx

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
)

// Buffer is a slog.Handler that holds records until Replay hands them to the real handler. The CLI logs into one
// while the configuration that selects the real handler (level, format, colours) is still being read, so those
// first records (such as "generated a random jwt.secret") get the same format, filtering and ring buffer as the
// rest. The zero value is not usable; call NewBuffer.
type Buffer struct {
	held *held
	ops  []bufferOp // WithAttrs / WithGroup calls, replayed in order
}

type held struct {
	mu   sync.Mutex
	recs []heldRecord
}

type heldRecord struct {
	ops []bufferOp
	rec slog.Record
}

type bufferOp struct {
	group string
	attrs []slog.Attr
}

// NewBuffer returns an empty Buffer.
func NewBuffer() *Buffer { return &Buffer{held: &held{}} }

// Enabled accepts every level: the real handler filters on Replay.
func (b *Buffer) Enabled(context.Context, slog.Level) bool { return true }

// Handle keeps a copy of r.
func (b *Buffer) Handle(_ context.Context, r slog.Record) error {
	b.held.mu.Lock()
	b.held.recs = append(b.held.recs, heldRecord{ops: b.ops, rec: r.Clone()})
	b.held.mu.Unlock()
	return nil
}

// WithAttrs records the attributes for Replay.
func (b *Buffer) WithAttrs(as []slog.Attr) slog.Handler {
	if len(as) == 0 {
		return b
	}
	return &Buffer{held: b.held, ops: append(slices.Clip(b.ops), bufferOp{attrs: as})}
}

// WithGroup records the group for Replay.
func (b *Buffer) WithGroup(name string) slog.Handler {
	if name == "" {
		return b
	}
	return &Buffer{held: b.held, ops: append(slices.Clip(b.ops), bufferOp{group: name})}
}

// Len returns the number of held records.
func (b *Buffer) Len() int {
	b.held.mu.Lock()
	defer b.held.mu.Unlock()
	return len(b.held.recs)
}

// Replay sends the held records, in order and with their original time, to h (after the same WithAttrs /
// WithGroup calls) when h is enabled for their level, and empties the buffer.
func (b *Buffer) Replay(ctx context.Context, h slog.Handler) error {
	b.held.mu.Lock()
	recs := b.held.recs
	b.held.recs = nil
	b.held.mu.Unlock()
	var errs []error
	for _, hr := range recs {
		target := h
		for _, op := range hr.ops {
			if op.group != "" {
				target = target.WithGroup(op.group)
			} else {
				target = target.WithAttrs(op.attrs)
			}
		}
		if target.Enabled(ctx, hr.rec.Level) {
			errs = append(errs, target.Handle(ctx, hr.rec))
		}
	}
	return errors.Join(errs...)
}
