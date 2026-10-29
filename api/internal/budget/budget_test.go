package budget

import (
	"context"
	"errors"
	"testing"
	"time"
)

func usd(v float64) *float64 { return &v }

func TestASessionStopsSpendingAtItsCapAndAClientAcrossSessionsAtItsOwn(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	m := New()
	m.now = func() time.Time { return now }
	client := func(ctx context.Context) context.Context { return m.With(ctx, "ip:1.2.3.4", 0.30) }
	a := client(m.With(context.Background(), "match:a", 0.25))

	for range 2 {
		if err := Check(a); err != nil {
			t.Fatalf("under the cap: %v", err)
		}
		Add(a, usd(0.12))
	}
	Add(a, nil) // a call that reported no cost
	if err := Check(a); err != nil {
		t.Fatalf("0.24 of 0.25: %v", err)
	}
	Add(a, usd(0.02))
	if err := Check(a); !errors.Is(err, ErrOverCap) {
		t.Fatalf("0.26 of 0.25: %v, want ErrOverCap", err)
	}
	if spent, capUSD, ok := Used(a); !ok || spent != 0.26 || capUSD != 0.25 {
		t.Fatalf("used %v of %v (%v), want 0.26 of 0.25", spent, capUSD, ok)
	}
	// A new session of the same client starts fresh but shares the client's cap.
	b := client(m.With(context.Background(), "match:b", 0.25))
	if err := Check(b); err != nil {
		t.Fatalf("a new session: %v", err)
	}
	Add(b, usd(0.05))
	if err := Check(b); !errors.Is(err, ErrOverCap) {
		t.Fatalf("client at 0.31 of 0.30: %v, want ErrOverCap", err)
	}
	// Another client, and a context without a meter, are not capped.
	if err := Check(m.With(context.Background(), "ip:5.6.7.8", 0.30)); err != nil {
		t.Fatalf("another client: %v", err)
	}
	if err := Check(context.Background()); err != nil {
		t.Fatal(err)
	}

	// The window ends: the spenders start again from zero.
	now = now.Add(Window + time.Second)
	if err := Check(a); err != nil {
		t.Fatalf("after the window: %v", err)
	}
	if spent, _, _ := Used(a); spent != 0 {
		t.Fatalf("spent %v after the window, want 0", spent)
	}
}
