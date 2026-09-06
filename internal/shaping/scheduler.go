package shaping

import (
	"math"
	"time"
)

const MaxBytes = 32 << 20
const MaxBucketBytes = 2 << 20
const MaxFlowBytes = 128 << 10
const MaxWait = 2 * time.Second

type Item struct {
	Value    any
	Bytes    int
	Flow     string
	Enqueued time.Time
}
type Bucket struct {
	rate      int64
	capacity  float64
	tokens    float64
	last      time.Time
	active    time.Time
	flows     map[string][]Item
	flowBytes map[string]int
	order     []string
	cursor    int
	bytes     int
}
type Scheduler struct {
	buckets map[string]*Bucket
	order   []string
	bytes   int
	cursor  int
}

func New() *Scheduler           { return &Scheduler{buckets: map[string]*Bucket{}} }
func (s *Scheduler) Bytes() int { return s.bytes }
func (s *Scheduler) Enqueue(key string, rate int64, item Item, now time.Time) bool {
	if rate <= 0 || item.Bytes <= 0 || item.Bytes > 65575 || s.bytes+item.Bytes > MaxBytes {
		return false
	}
	b := s.buckets[key]
	if b == nil {
		cap := math.Max(65575, float64(rate)*0.02)
		b = &Bucket{rate: rate, capacity: cap, tokens: cap, last: now, flows: map[string][]Item{}, flowBytes: map[string]int{}}
		s.buckets[key] = b
		s.order = append(s.order, key)
	}
	if b.bytes+item.Bytes > MaxBucketBytes || b.flowBytes[item.Flow]+item.Bytes > MaxFlowBytes {
		return false
	}
	b.active = now
	if _, ok := b.flows[item.Flow]; !ok {
		b.order = append(b.order, item.Flow)
	}
	item.Enqueued = now
	b.flows[item.Flow] = append(b.flows[item.Flow], item)
	b.bytes += item.Bytes
	b.flowBytes[item.Flow] += item.Bytes
	s.bytes += item.Bytes
	return true
}

// Drain visits each rule and flow in round-robin order. One owner goroutine calls it.
func (s *Scheduler) Drain(now time.Time, send func(Item), drop func(Item)) time.Duration {
	next := time.Second
	for n := 0; n < len(s.order); n++ {
		idx := (s.cursor + n) % len(s.order)
		b := s.buckets[s.order[idx]]
		elapsed := now.Sub(b.last).Seconds()
		if elapsed > 0 {
			b.tokens = math.Min(b.capacity, b.tokens+elapsed*float64(b.rate))
			b.last = now
		}
		for visits := 0; len(b.order) > 0 && visits < 4096; visits++ {
			if b.cursor >= len(b.order) {
				b.cursor = 0
			}
			flow := b.order[b.cursor]
			q := b.flows[flow]
			item := q[0]
			expired := now.Sub(item.Enqueued) >= MaxWait
			if !expired && b.tokens < float64(item.Bytes) {
				d := time.Duration((float64(item.Bytes) - b.tokens) / float64(b.rate) * float64(time.Second))
				if remaining := MaxWait - now.Sub(item.Enqueued); remaining < d {
					d = remaining
				}
				if d < next {
					next = d
				}
				// Keep the blocked head to avoid starvation by smaller packets.
				break
			}
			if expired {
				drop(item)
			} else {
				b.tokens -= float64(item.Bytes)
				send(item)
			}
			b.bytes -= item.Bytes
			b.flowBytes[flow] -= item.Bytes
			s.bytes -= item.Bytes
			q[0] = Item{}
			q = q[1:]
			if len(q) == 0 {
				delete(b.flows, flow)
				delete(b.flowBytes, flow)
				b.order = append(b.order[:b.cursor], b.order[b.cursor+1:]...)
			} else {
				b.flows[flow] = q
				if !expired {
					b.cursor++
				}
			}
		}
		if b.bytes > 0 && b.tokens >= 65575 && next > time.Millisecond {
			next = time.Millisecond
		}
	}
	if len(s.order) > 0 {
		s.cursor = (s.cursor + 1) % len(s.order)
	}
	if next < time.Millisecond {
		next = time.Millisecond
	}
	return next
}
func (s *Scheduler) Flush(send func(Item)) {
	for _, key := range s.order {
		b := s.buckets[key]
		for _, flow := range b.order {
			for _, item := range b.flows[flow] {
				send(item)
			}
		}
	}
	*s = *New()
}

// Configure updates existing budgets without releasing unrelated queued traffic.
func (s *Scheduler) Configure(rates map[string]int64, now time.Time, send func(Item)) {
	out := s.order[:0]
	for _, key := range s.order {
		b := s.buckets[key]
		rate, ok := rates[key]
		if !ok || rate <= 0 {
			for _, flow := range b.order {
				for _, item := range b.flows[flow] {
					send(item)
					s.bytes -= item.Bytes
				}
			}
			delete(s.buckets, key)
			continue
		}
		b.tokens = math.Min(b.capacity, b.tokens+math.Max(0, now.Sub(b.last).Seconds())*float64(b.rate))
		b.rate = rate
		b.capacity = math.Max(65575, float64(rate)*0.02)
		b.tokens = math.Min(b.tokens, b.capacity)
		b.last = now
		out = append(out, key)
	}
	s.order = out
}

// Sweep preserves idle buckets briefly so intermittent connections cannot reset bursts.
func (s *Scheduler) Sweep(now time.Time) {
	out := s.order[:0]
	for _, key := range s.order {
		b := s.buckets[key]
		if b.bytes == 0 && now.Sub(b.active) > time.Minute {
			delete(s.buckets, key)
		} else {
			out = append(out, key)
		}
	}
	s.order = out
}
