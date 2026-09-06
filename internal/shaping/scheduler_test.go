package shaping

import (
	"fmt"
	"testing"
	"time"
)

func TestSharedBudgetAcrossFlows(t *testing.T) {
	s := New()
	start := time.Unix(100, 0)
	sent := map[string]int{}
	dropped := 0
	for ms := 0; ms < 30000; ms++ {
		now := start.Add(time.Duration(ms) * time.Millisecond)
		for f := 0; f < 4; f++ {
			flow := fmt.Sprint(f)
			s.Enqueue("application:down", 100000, Item{Flow: flow, Bytes: 500}, now)
		}
		s.Drain(now, func(i Item) { sent[i.Flow] += i.Bytes }, func(i Item) { dropped++ })
	}
	total := 0
	for _, n := range sent {
		total += n
	}
	if total > 3_000_000+65575 || total < 2_990_000 {
		t.Fatalf("shared budget incorrect: %d bytes", total)
	}
	for flow, n := range sent {
		if n < total/4-1000 || n > total/4+1000 {
			t.Fatalf("flow %s starved: %d of %d", flow, n, total)
		}
	}
	if dropped == 0 {
		t.Fatal("overload should expire queued packets")
	}
	if s.Bytes() > MaxBytes {
		t.Fatal("queue exceeded cap")
	}
}

func TestDirectionsIndependentAndFlowOrder(t *testing.T) {
	s := New()
	now := time.Unix(100, 0)
	order := []int{}
	for i := 0; i < 50; i++ {
		s.Enqueue("app:down", 1000, Item{Flow: "one", Bytes: 1500, Value: i}, now)
	}
	s.Enqueue("app:up", 1000, Item{Flow: "up", Bytes: 60000, Value: -1}, now)
	upload := false
	s.Drain(now, func(i Item) {
		if i.Value.(int) == -1 {
			upload = true
		} else {
			order = append(order, i.Value.(int))
		}
	}, func(Item) { t.Fatal("unexpected drop") })
	if !upload {
		t.Fatal("upload was blocked by download budget")
	}
	for i, n := range order {
		if i != n {
			t.Fatalf("reordered %v", order)
		}
	}
	if len(order) != 43 {
		t.Fatalf("unexpected initial burst: %d", len(order))
	}
}
func TestQueueBoundsExpiryAndFlush(t *testing.T) {
	s := New()
	now := time.Now()
	accepted := 0
	for i := 0; i < 100; i++ {
		if s.Enqueue("a", 1, Item{Flow: "a", Bytes: 65575}, now) {
			accepted++
		}
	}
	if accepted*65575 > MaxBucketBytes {
		t.Fatal("per-rule cap exceeded")
	}
	dropped := 0
	s.Drain(now.Add(MaxWait), func(Item) { t.Fatal("expired packet sent") }, func(Item) { dropped++ })
	if dropped != accepted || s.Bytes() != 0 {
		t.Fatal("expired memory retained")
	}
	s.Enqueue("b", 1000, Item{Flow: "b", Bytes: 1000}, now)
	flushed := 0
	s.Flush(func(Item) { flushed++ })
	if flushed != 1 || s.Bytes() != 0 {
		t.Fatal("flush did not clear queue")
	}
}
func TestIdleBudgetRetainedAndReclaimed(t *testing.T) {
	s := New()
	now := time.Now()
	send := func(Item) {}
	s.Enqueue("a", 1, Item{Flow: "a", Bytes: 65575}, now)
	s.Drain(now, send, send)
	s.Enqueue("a", 1, Item{Flow: "b", Bytes: 1000}, now.Add(time.Second))
	count := 0
	s.Drain(now.Add(time.Second), func(Item) { count++ }, send)
	if count != 0 {
		t.Fatal("new flow incorrectly reset budget")
	}
	s.Drain(now.Add(4*time.Second), send, send)
	s.Sweep(now.Add(70 * time.Second))
	if len(s.buckets) != 0 {
		t.Fatal("idle bucket not reclaimed")
	}
}

func TestRuleEditDoesNotFlushUnrelatedPackets(t *testing.T) {
	s := New()
	now := time.Now()
	sent := 0
	send := func(Item) { sent++ }
	s.Enqueue("a", 1000, Item{Flow: "a", Bytes: 65000}, now)
	s.Drain(now, send, send)
	s.Enqueue("a", 1000, Item{Flow: "a", Bytes: 65000}, now)
	s.Enqueue("b", 1000, Item{Flow: "b", Bytes: 65000}, now)
	s.Configure(map[string]int64{"a": 500}, now, send)
	if sent != 2 || s.Bytes() != 65000 {
		t.Fatal("editing rules released an unrelated queue")
	}
	s.Drain(now.Add(time.Second), send, send)
	if sent != 2 {
		t.Fatal("rate edit reset burst budget")
	}
}
