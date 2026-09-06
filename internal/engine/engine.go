package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"speedlimitfree/internal/attribution"
	"speedlimitfree/internal/contracts"
	"speedlimitfree/internal/processes"
	"speedlimitfree/internal/rules"
	"speedlimitfree/internal/shaping"
	"speedlimitfree/internal/storage"
	"speedlimitfree/internal/traffic"
	"sync"
	"sync/atomic"
	"time"
)

type counter struct {
	down, up                 uint64
	previousDown, previousUp uint64
	download, upload         float64
	seen                     time.Time
}
type Engine struct {
	configMu         sync.Mutex
	mu               sync.RWMutex
	settings         contracts.Settings
	index            *rules.Index
	path             string
	state, message   string
	processes        []contracts.Process
	counters         map[string]*counter
	download, upload float64
	started          time.Time
	table            *attribution.Table
	changed          chan struct{}
	unknown, dropped atomic.Uint64
	queued           atomic.Int64
}

func New(path string) (*Engine, error) {
	s, err := storage.Load(path)
	if err != nil {
		return nil, err
	}
	return &Engine{settings: s, index: rules.Compile(s.Rules), path: path, state: "starting", message: "Starting traffic engine", processes: []contracts.Process{}, counters: map[string]*counter{}, started: time.Now(), table: attribution.New(), changed: make(chan struct{}, 1)}, nil
}
func (e *Engine) setState(state, message string) {
	e.mu.Lock()
	e.state = state
	e.message = message
	e.mu.Unlock()
}
func (e *Engine) Run(ctx context.Context, driverDir string, monitor bool) (runErr error) {
	ctx, cancel := context.WithCancel(ctx)
	e.refresh()
	refreshDone := make(chan struct{})
	defer func() {
		cancel()
		<-refreshDone
		if runErr != nil {
			e.setState("unavailable", runErr.Error())
		}
	}()
	go func() {
		defer close(refreshDone)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		rates := time.NewTicker(500 * time.Millisecond)
		defer rates.Stop()
		last := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.refresh()
			case now := <-rates.C:
				e.sample(now.Sub(last).Seconds())
				last = now
			}
		}
	}()
	if monitor {
		e.setState("monitor", "Process discovery only. Traffic interception is disabled.")
		<-ctx.Done()
		<-refreshDone
		return nil
	}
	d, err := traffic.Open(driverDir)
	if err != nil {
		e.setState("unavailable", err.Error())
		log.Printf("traffic engine unavailable: %v", err)
		<-ctx.Done()
		<-refreshDone
		return nil
	}
	defer d.Close()
	e.setState("running", "Traffic engine connected")
	packets := make(chan *traffic.Packet, 256)
	fail := make(chan error, 3)
	var workers sync.WaitGroup
	for _, socket := range []bool{false, true} {
		workers.Add(1)
		go func(socket bool) {
			defer workers.Done()
			err := d.Observe(socket, e.table.Event)
			select {
			case fail <- err:
			default:
			}
		}(socket)
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		defer close(packets)
		for {
			p, err := d.Recv()
			if err != nil {
				select {
				case fail <- err:
				default:
				}
				return
			}
			select {
			case packets <- p:
			case <-ctx.Done():
				d.Send(p)
				d.Release(p)
				return
			}
		}
	}()
	scheduler := shaping.New()
	send := func(item shaping.Item) {
		p := item.Value.(*ownedPacket)
		if err := d.Send(p.packet); err != nil {
			e.dropped.Add(1)
			select {
			case fail <- fmt.Errorf("packet injection failed: %w", err):
			default:
			}
		} else {
			e.count(p.process, p.packet)
		}
		d.Release(p.packet)
	}
	drop := func(item shaping.Item) { e.dropped.Add(1); d.Release(item.Value.(*ownedPacket).packet) }
	timer := time.NewTimer(time.Millisecond)
	defer timer.Stop()
	defer func() {
		d.StopReceive()
		scheduler.Flush(send)
		d.Close()
		for p := range packets {
			d.Release(p)
		}
		workers.Wait()
		e.queued.Store(0)
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-fail:
			e.setState("unavailable", fmt.Sprintf("Traffic interception stopped: %v", err))
			return err
		case <-e.changed:
			e.mu.RLock()
			rates := map[string]int64{}
			if !e.settings.Paused {
				for _, r := range e.settings.Rules {
					if !r.Enabled {
						continue
					}
					if r.Download != nil {
						rates[r.ID+":down"] = *r.Download
					}
					if r.Upload != nil {
						rates[r.ID+":up"] = *r.Upload
					}
				}
			}
			e.mu.RUnlock()
			scheduler.Configure(rates, time.Now(), send)
		case now := <-timer.C:
			next := scheduler.Drain(now, send, drop)
			e.queued.Store(int64(scheduler.Bytes()))
			scheduler.Sweep(now)
			timer.Reset(next)
		case p, ok := <-packets:
			if !ok {
				return errors.New("packet stream closed")
			}
			process, known := e.table.Lookup(p.Key)
			known = known && p.Known
			if !known {
				e.unknown.Add(uint64(len(p.Data)))
				if err := d.Send(p); err != nil {
					d.Release(p)
					return err
				}
				d.Release(p)
				continue
			}
			e.mu.RLock()
			paused := e.settings.Paused
			rule := e.index.Lookup(process)
			var rate *int64
			key := ""
			if rule != nil {
				rate = rule.Download
				key = rule.ID + ":down"
				if p.Outbound {
					rate = rule.Upload
					key = rule.ID + ":up"
				}
			}
			e.mu.RUnlock()
			item := shaping.Item{Value: &ownedPacket{p, process}, Bytes: len(p.Data)}
			if paused || rate == nil {
				send(item)
				continue
			}
			item.Flow = fmt.Sprintf("%d/%s/%s", p.Key.Protocol, p.Key.Local, p.Key.Remote)
			idle := scheduler.Bytes() == 0
			if !scheduler.Enqueue(key, *rate, item, time.Now()) {
				drop(item)
			}
			if idle {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(time.Millisecond)
			}
		}
	}
}

type ownedPacket struct {
	packet  *traffic.Packet
	process contracts.Process
}

func (e *Engine) count(p contracts.Process, packet *traffic.Packet) {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := rules.Identity(p)
	c := e.counters[key]
	if c == nil {
		c = &counter{}
		e.counters[key] = c
	}
	c.seen = time.Now()
	if packet.Outbound {
		c.up += uint64(len(packet.Data))
	} else {
		c.down += uint64(len(packet.Data))
	}
}
func (e *Engine) sample(seconds float64) {
	if seconds <= 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.download = 0
	e.upload = 0
	for key, c := range e.counters {
		c.download = float64(c.down-c.previousDown) / seconds
		c.upload = float64(c.up-c.previousUp) / seconds
		c.previousDown = c.down
		c.previousUp = c.up
		e.download += c.download
		e.upload += c.upload
		if time.Since(c.seen) > time.Minute {
			delete(e.counters, key)
		}
	}
}
func (e *Engine) refresh() {
	ps, err := processes.List()
	if err != nil {
		return
	}
	e.table.Refresh(ps)
	e.configMu.Lock()
	defer e.configMu.Unlock()
	e.mu.Lock()
	e.processes = ps
	live := map[string]bool{}
	for _, p := range ps {
		live[rules.Identity(p)] = true
	}
	out := make([]contracts.Rule, 0, len(e.settings.Rules))
	for _, r := range e.settings.Rules {
		if r.Scope == "application" || live[fmt.Sprintf("%d:%s", r.PID, r.Started)] {
			out = append(out, r)
		}
	}
	changed := len(out) != len(e.settings.Rules)
	e.settings.Rules = out
	if changed {
		e.index = rules.Compile(out)
	}
	e.mu.Unlock()
	if changed {
		e.notify()
	}
}
func (e *Engine) Snapshot() contracts.Snapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()
	ps := append([]contracts.Process{}, e.processes...)
	rs := append([]contracts.Rule{}, e.settings.Rules...)
	for i := range ps {
		p := &ps[i]
		if c := e.counters[rules.Identity(*p)]; c != nil {
			p.Download = c.download
			p.Upload = c.upload
			p.Downloaded = c.down
			p.Uploaded = c.up
		}
		if r := e.index.Lookup(*p); r != nil {
			p.RuleID = r.ID
		}
	}
	return contracts.Snapshot{Version: contracts.Version, Connected: true, Engine: e.state, Message: e.message, Paused: e.settings.Paused, Processes: ps, Rules: rs, Download: e.download, Upload: e.upload, UnknownBytes: e.unknown.Load(), Dropped: e.dropped.Load(), QueueBytes: e.queued.Load(), Uptime: int64(time.Since(e.started).Seconds())}
}
func (e *Engine) notify() {
	select {
	case e.changed <- struct{}{}:
	default:
	}
}
func (e *Engine) Command(req contracts.Request) error {
	e.configMu.Lock()
	defer e.configMu.Unlock()
	e.mu.Lock()
	next := e.settings
	next.Rules = append([]contracts.Rule{}, next.Rules...)
	e.mu.Unlock()
	switch req.Method {
	case "save":
		if req.Rule == nil {
			return errors.New("rule is required")
		}
		r := *req.Rule
		if err := rules.Validate(r); err != nil {
			return err
		}
		if r.Scope == "process" {
			p, err := processes.Get(r.PID)
			if err != nil || p.Started != r.Started || rules.Path(p.Path) != rules.Path(r.Path) {
				return errors.New("this process has exited or changed; refresh the list")
			}
		}
		if r.ID == "" {
			r.ID = rules.NewID()
		}
		found := false
		for i, old := range next.Rules {
			if old.ID == r.ID {
				next.Rules[i] = r
				found = true
			} else if rules.SameTarget(old, r) {
				return errors.New("a rule already exists for this target; edit that rule")
			}
		}
		if !found {
			if len(next.Rules) >= 1000 {
				return errors.New("maximum of 1000 rules reached")
			}
			next.Rules = append(next.Rules, r)
		}
	case "delete":
		found := false
		for i, r := range next.Rules {
			if r.ID == req.ID {
				next.Rules = append(next.Rules[:i], next.Rules[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			return errors.New("rule no longer exists")
		}
	case "pause":
		next.Paused = req.Paused
	default:
		return errors.New("unknown command")
	}
	if err := storage.Save(e.path, next); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	e.mu.Lock()
	e.settings = next
	e.index = rules.Compile(next.Rules)
	e.mu.Unlock()
	e.notify()
	return nil
}
