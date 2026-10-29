// Package budget caps what model calls may spend per match session and per
// client (an anonymous client's IP, or a signed-in account). Costs are the
// providers' usage reports, the figures model_call and decision_log keep.
// Past a cap, calls stop and the code ranking and templates answer
// (ADR-0003: fail open).
package budget

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"
)

// ErrOverCap means a spender used its cap; no model call is made.
var ErrOverCap = errors.New("budget: the model spend cap is used up")

// Window is how long a spend counts; then the spender starts from zero.
const Window = 24 * time.Hour

// maxSpenders bounds the memory: past it, finished windows are dropped.
const maxSpenders = 10_000

type spend struct {
	micros int64 // USD millionths, so sums stay exact
	since  time.Time
}

// Meter keeps every spender's spend for the current window, in memory.
// ponytail: one process, lost on restart; a shared store when serve scales out.
type Meter struct {
	mu    sync.Mutex
	spent map[string]*spend
	now   func() time.Time
}

// New builds an empty meter.
func New() *Meter { return &Meter{spent: map[string]*spend{}, now: time.Now} }

func micros(usd float64) int64 { return int64(math.Round(usd * 1e6)) }

type key struct{}

type tab struct {
	id        string
	capMicros int64
}

type tabs struct {
	m    *Meter
	list []tab
}

// With charges ctx's calls to spender id as well, up to capUSD; a cap of 0
// or less leaves the spender uncapped. The first spender is the one Used reports.
func (m *Meter) With(ctx context.Context, id string, capUSD float64) context.Context {
	if m == nil || id == "" || capUSD <= 0 {
		return ctx
	}
	cur, _ := ctx.Value(key{}).(tabs)
	list := append(append([]tab{}, cur.list...), tab{id, micros(capUSD)})
	return context.WithValue(ctx, key{}, tabs{m, list})
}

// get is the spender's live window, opened when missing or over.
func (m *Meter) get(id string) *spend {
	now := m.now()
	s := m.spent[id]
	if s == nil || now.Sub(s.since) >= Window {
		if len(m.spent) >= maxSpenders {
			for k, v := range m.spent {
				if now.Sub(v.since) >= Window {
					delete(m.spent, k)
				}
			}
		}
		s = &spend{since: now}
		m.spent[id] = s
	}
	return s
}

// Check says whether a model call may run on ctx: every spender is under its cap.
func Check(ctx context.Context) error {
	t, ok := ctx.Value(key{}).(tabs)
	if !ok {
		return nil
	}
	t.m.mu.Lock()
	defer t.m.mu.Unlock()
	for _, tb := range t.list {
		if t.m.get(tb.id).micros >= tb.capMicros {
			return ErrOverCap
		}
	}
	return nil
}

// Add charges a call's reported cost to every spender; nil, when the
// provider reported none, charges nothing.
func Add(ctx context.Context, usd *float64) {
	t, ok := ctx.Value(key{}).(tabs)
	if !ok || usd == nil || *usd <= 0 {
		return
	}
	t.m.mu.Lock()
	defer t.m.mu.Unlock()
	for _, tb := range t.list {
		t.m.get(tb.id).micros += micros(*usd)
	}
}

// Used is the first spender's spend and cap in USD; ok is false without one.
func Used(ctx context.Context) (spent, capUSD float64, ok bool) {
	t, ok := ctx.Value(key{}).(tabs)
	if !ok {
		return 0, 0, false
	}
	t.m.mu.Lock()
	defer t.m.mu.Unlock()
	tb := t.list[0]
	return float64(t.m.get(tb.id).micros) / 1e6, float64(tb.capMicros) / 1e6, true
}
