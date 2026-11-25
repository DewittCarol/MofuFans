// Package guard provides bounded application-level abuse protection for one API instance.
package guard

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const maxKeys = 16384
const maxBans = 4096
const maxEvents = 1000
const retention = 7 * 24 * time.Hour

type Policy struct {
	PerMinute       int `json:"per_minute"`
	Burst           int `json:"burst"`
	GlobalPerMinute int `json:"global_per_minute"`
	Concurrent      int `json:"concurrent"`
}

func DefaultPolicies() map[string]Policy {
	return map[string]Policy{
		"read":         {600, 120, 12000, 64},
		"download":     {600, 120, 12000, 32},
		"write":        {120, 30, 1800, 16},
		"post":         {10, 5, 120, 4},
		"reaction":     {120, 20, 3000, 16},
		"upload":       {240, 32, 480, 4},
		"admin-login":  {3, 8, 60, 2},
		"member-login": {3, 10, 120, 4},
		"claim":        {12, 12, 300, 8},
		"signature":    {12, 12, 180, 4},
		"upstream":     {30, 10, 120, 4},
		"stream":       {12, 12, 600, 64},
	}
}

type Options struct {
	Directory string
	Secret    string
	Now       func() time.Time
	Policies  map[string]Policy
}
type Ban struct {
	ID     string    `json:"id"`
	Kind   string    `json:"kind"`
	IP     string    `json:"ip,omitempty"`
	Target string    `json:"target,omitempty"`
	Scope  string    `json:"scope"`
	Reason string    `json:"reason"`
	Until  time.Time `json:"until"`
}
type Event struct {
	ID      string    `json:"id"`
	At      time.Time `json:"at"`
	FirstAt time.Time `json:"first_at"`
	Kind    string    `json:"kind"`
	IP      string    `json:"ip"`
	Scope   string    `json:"scope"`
	Target  string    `json:"target,omitempty"`
	Count   int       `json:"count"`
}
type failure struct {
	Count   int       `json:"count"`
	Reset   time.Time `json:"reset"`
	Strikes int       `json:"strikes"`
	Expires time.Time `json:"expires"`
}
type diskState struct {
	Version  int                `json:"version"`
	Bans     map[string]Ban     `json:"bans"`
	Failures map[string]failure `json:"failures"`
	Events   []Event            `json:"events"`
}
type bucket struct {
	limiter *rate.Limiter
	seen    time.Time
}
type Manager struct {
	mu        sync.Mutex
	opts      Options
	state     diskState
	buckets   map[string]*bucket
	active    map[string]int
	streamIPs map[string]int
	streams   int
	lastSweep time.Time
	dirty     bool
	lastError bool
	lock      *os.File
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	closed    bool
}

func CanonicalIP(raw string) (string, error) {
	ip, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil || ip.Zone() != "" {
		return "", errors.New("invalid IP address")
	}
	return ip.Unmap().String(), nil
}
func validScope(scope string) bool { _, ok := DefaultPolicies()[scope]; return ok || scope == "all" }
func newID() string {
	var b [12]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func New(opts Options) (*Manager, error) {
	if opts.Directory == "" || len(opts.Secret) < 32 {
		return nil, errors.New("security state directory and secret required")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	policies := DefaultPolicies()
	for k, p := range opts.Policies {
		if _, ok := policies[k]; !ok || p.PerMinute < 1 || p.PerMinute > 60000 || p.Burst < 1 || p.Burst > 1000 || p.GlobalPerMinute < 1 || p.GlobalPerMinute > 120000 || p.Concurrent < 1 || p.Concurrent > 128 {
			return nil, errors.New("invalid security policy")
		}
		policies[k] = p
	}
	opts.Policies = policies
	lock, err := openLock(opts.Directory)
	if err != nil {
		return nil, err
	}
	g := &Manager{opts: opts, lock: lock, buckets: map[string]*bucket{}, active: map[string]int{}, streamIPs: map[string]int{}, stop: make(chan struct{}), done: make(chan struct{})}
	if err = g.load(); err != nil {
		releaseLock(lock)
		return nil, err
	}
	g.sweepLocked()
	go func() {
		defer close(g.done)
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				g.mu.Lock()
				g.sweepLocked()
				if g.dirty {
					_ = g.flushLocked()
				}
				g.mu.Unlock()
			case <-g.stop:
				g.mu.Lock()
				if g.dirty {
					_ = g.flushLocked()
				}
				g.mu.Unlock()
				return
			}
		}
	}()
	return g, nil
}
func (g *Manager) Close() {
	g.closeOnce.Do(func() {
		close(g.stop)
		<-g.done
		g.mu.Lock()
		g.closed = true
		if g.dirty {
			_ = g.flushLocked()
		}
		g.mu.Unlock()
		releaseLock(g.lock)
	})
}
func (g *Manager) now() time.Time { return g.opts.Now().UTC() }
func (g *Manager) target(scope, value string) string {
	h := hmac.New(sha256.New, []byte(g.opts.Secret))
	_, _ = io.WriteString(h, scope+"\x00"+value)
	return hex.EncodeToString(h.Sum(nil))
}
func (g *Manager) load() error {
	g.state = diskState{Version: 1, Bans: map[string]Ban{}, Failures: map[string]failure{}, Events: []Event{}}
	path := filepath.Join(g.opts.Directory, "state.json")
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Size() > 8<<20) {
		return errors.New("invalid security state file")
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, (8<<20)+1))
	d.DisallowUnknownFields()
	if d.Decode(&g.state) != nil || d.Decode(new(any)) != io.EOF || g.state.Version != 1 || len(g.state.Bans) > maxBans || len(g.state.Failures) > maxKeys || len(g.state.Events) > maxEvents {
		return errors.New("invalid security state; restore or use the offline recovery command")
	}
	if g.state.Bans == nil {
		g.state.Bans = map[string]Ban{}
	}
	if g.state.Failures == nil {
		g.state.Failures = map[string]failure{}
	}
	for id, b := range g.state.Bans {
		if id != b.ID || len(id) != 24 || !validScope(b.Scope) || (b.Kind != "ip" && b.Kind != "target") || (b.Kind == "ip" && b.IP == "") || (b.Kind == "target" && len(b.Target) != 64) {
			return errors.New("invalid security ban")
		}
		if b.IP != "" {
			if ip, e := CanonicalIP(b.IP); e != nil || ip != b.IP {
				return errors.New("invalid security ban IP")
			}
		}
	}
	return nil
}
func (g *Manager) flushLocked() error {
	data, err := json.Marshal(g.state)
	if err == nil {
		err = writeAtomic(g.opts.Directory, data)
	}
	g.lastError = err != nil
	if err == nil {
		g.dirty = false
	}
	return err
}
func writeAtomic(directory string, data []byte) error {
	f, err := os.CreateTemp(directory, ".state-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(directory, "state.json")); err != nil {
		return err
	}
	if dir, e := os.Open(directory); e == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
func (g *Manager) sweepLocked() {
	now := g.now()
	if !g.lastSweep.IsZero() && now.Sub(g.lastSweep) < time.Minute {
		return
	}
	g.lastSweep = now
	for k, b := range g.state.Bans {
		if !now.Before(b.Until) {
			delete(g.state.Bans, k)
			g.dirty = true
		}
	}
	for k, f := range g.state.Failures {
		if !now.Before(f.Expires) {
			delete(g.state.Failures, k)
			g.dirty = true
		}
	}
	for k, b := range g.buckets {
		if now.Sub(b.seen) > 20*time.Minute {
			delete(g.buckets, k)
		}
	}
	kept := g.state.Events[:0]
	for _, e := range g.state.Events {
		if now.Sub(e.At) < retention {
			kept = append(kept, e)
		}
	}
	if len(kept) != len(g.state.Events) {
		g.dirty = true
	}
	g.state.Events = kept
}
func (g *Manager) eventLocked(kind, ip, scope, target string) {
	if g.closed {
		return
	}
	now := g.now()
	// Coalesce bursts; logs contain only fixed classifications, canonical IPs and keyed target hashes.
	for i := len(g.state.Events) - 1; i >= 0 && i >= len(g.state.Events)-64; i-- {
		e := &g.state.Events[i]
		if e.Kind == kind && e.IP == ip && e.Scope == scope && e.Target == target && now.Sub(e.FirstAt) < time.Minute {
			e.At = now
			if e.Count < 1000000000 {
				e.Count++
			}
			g.dirty = true
			return
		}
	}
	if len(g.state.Events) >= maxEvents {
		copy(g.state.Events, g.state.Events[1:])
		g.state.Events = g.state.Events[:maxEvents-1]
	}
	g.state.Events = append(g.state.Events, Event{ID: newID(), At: now, FirstAt: now, Kind: kind, IP: ip, Scope: scope, Target: target, Count: 1})
	g.dirty = true
}
func (g *Manager) blockedLocked(ip, scope, target string) int {
	after := 0
	now := g.now()
	for _, b := range g.state.Bans {
		if !now.Before(b.Until) || (b.Scope != "all" && b.Scope != scope) {
			continue
		}
		if (b.Kind == "ip" && b.IP == ip) || (b.Kind == "target" && target != "" && b.Target == target) {
			seconds := int(b.Until.Sub(now).Seconds()) + 1
			if seconds > after {
				after = seconds
			}
		}
	}
	return after
}
func (g *Manager) takeLocked(key string, perMinute, burst int) (bool, int) {
	now := g.now()
	b := g.buckets[key]
	if b == nil {
		if len(g.buckets) >= maxKeys {
			return false, 60
		}
		b = &bucket{limiter: rate.NewLimiter(rate.Limit(float64(perMinute)/60), burst)}
		g.buckets[key] = b
	}
	b.seen = now
	if b.limiter.AllowN(now, 1) {
		return true, 0
	}
	return false, int(60/float64(perMinute)) + 1
}

// Enter reserves both a rate budget and a concurrency slot without waiting.
func (g *Manager) Enter(rawIP, scope string) (func(), int) {
	ip, err := CanonicalIP(rawIP)
	if err != nil {
		return nil, 60
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweepLocked()
	if g.closed {
		return nil, 5
	}
	p, ok := g.opts.Policies[scope]
	if !ok {
		return nil, 60
	}
	if g.lastError && scope != "read" && scope != "stream" {
		return nil, 5
	}
	if after := g.blockedLocked(ip, scope, ""); after > 0 {
		g.eventLocked("blocked", ip, scope, "")
		return nil, after
	}
	// Already-throttled identities must not spend everybody else's global budget.
	ipBudgetKey := "ip:" + scope + ":" + ip
	ipBudgetTaken := false
	if b := g.buckets[ipBudgetKey]; b != nil {
		b.seen = g.now()
		if !b.limiter.AllowN(g.now(), 1) {
			g.eventLocked("rate_limit", ip, scope, "")
			g.abuseLocked(ip, scope)
			return nil, int(60/float64(p.PerMinute)) + 1
		}
		ipBudgetTaken = true
	}
	// A single IPv6 /64 cannot manufacture thousands of independent visitor budgets.
	// This is a rate budget, never a subnet ban or an IP allowlist.
	address, _ := netip.ParseAddr(ip)
	if address.Is6() {
		prefix, _ := address.Prefix(64)
		if ok, after := g.takeLocked("prefix:"+scope+":"+prefix.Masked().String(), p.PerMinute, p.Burst); !ok {
			g.eventLocked("rate_limit", ip, scope, "")
			return nil, after
		}
	}
	if ok, after := g.takeLocked("global:"+scope, p.GlobalPerMinute, max(p.Burst, p.GlobalPerMinute/60)); !ok {
		g.eventLocked("global_limit", ip, scope, "")
		return nil, after
	}
	if !ipBudgetTaken {
		if ok, after := g.takeLocked(ipBudgetKey, p.PerMinute, p.Burst); !ok {
			g.eventLocked("rate_limit", ip, scope, "")
			g.abuseLocked(ip, scope)
			return nil, after
		}
	}
	ipKey := "active:" + scope + ":" + ip
	if g.active[scope] >= p.Concurrent || g.active[ipKey] >= min(p.Concurrent, 8) {
		g.eventLocked("busy", ip, scope, "")
		return nil, 2
	}
	g.active[scope]++
	g.active[ipKey]++
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			g.active[scope]--
			g.active[ipKey]--
			if g.active[ipKey] == 0 {
				delete(g.active, ipKey)
			}
			g.mu.Unlock()
		})
	}, 0
}

type Attempt struct {
	manager           *Manager
	ip, scope, target string
	once              sync.Once
}

func (g *Manager) Begin(rawIP, scope, identity string) (*Attempt, int) {
	ip, err := CanonicalIP(rawIP)
	if err != nil || !validScope(scope) || scope == "all" {
		return nil, 60
	}
	target := g.target(scope, identity)
	g.mu.Lock()
	defer g.mu.Unlock()
	if after := g.blockedLocked(ip, scope, target); after > 0 {
		g.eventLocked("blocked", ip, scope, target)
		return nil, after
	}
	if g.lastError {
		return nil, 5
	}
	if g.closed {
		return nil, 5
	}
	if g.active["target:"+target] > 0 {
		return nil, 2
	}
	g.active["target:"+target]++
	return &Attempt{manager: g, ip: ip, scope: scope, target: target}, 0
}
func (a *Attempt) release() {
	a.manager.mu.Lock()
	delete(a.manager.active, "target:"+a.target)
	a.manager.mu.Unlock()
}
func (a *Attempt) Cancel() {
	if a != nil && a.manager != nil {
		a.once.Do(a.release)
	}
}
func (a *Attempt) Success() {
	if a != nil && a.manager != nil {
		a.once.Do(func() { a.manager.result(a, true); a.release() })
	}
}
func (a *Attempt) Failure() {
	if a != nil && a.manager != nil {
		a.once.Do(func() { a.manager.result(a, false); a.release() })
	}
}
func (g *Manager) result(a *Attempt, success bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweepLocked()
	if g.closed {
		return
	}
	if success {
		delete(g.state.Failures, "target:"+a.scope+":"+a.target)
		g.eventLocked("auth_success", a.ip, a.scope, a.target)
		return // A valid account must not erase the IP's failures against other accounts.
	}
	g.eventLocked("auth_failure", a.ip, a.scope, a.target)
	newBan := false
	for _, kind := range []string{"ip", "target"} {
		key := kind + ":" + a.scope + ":" + a.ip
		threshold := 8
		if kind == "target" {
			key = kind + ":" + a.scope + ":" + a.target
			threshold = 10
		}
		f, exists := g.state.Failures[key]
		if !exists && len(g.state.Failures) >= maxKeys {
			continue
		}
		now := g.now()
		if !now.Before(f.Reset) {
			f.Count = 0
			f.Reset = now.Add(10 * time.Minute)
		}
		f.Count++
		f.Expires = now.Add(24 * time.Hour)
		if f.Count >= threshold && len(g.state.Bans) < maxBans {
			f.Count = 0
			f.Strikes = min(f.Strikes+1, 4)
			duration := time.Duration(15*(1<<uint(f.Strikes-1))) * time.Minute
			if kind == "target" {
				duration = time.Duration(30*f.Strikes) * time.Second
			} else if duration > time.Hour {
				duration = time.Hour
			}
			b := Ban{ID: newID(), Kind: kind, Scope: a.scope, Reason: "auth_failures", Until: now.Add(duration)}
			if kind == "ip" {
				b.IP = a.ip
			} else {
				b.Target = a.target
			}
			g.state.Bans[b.ID] = b
			newBan = true
			g.eventLocked("auto_ban", a.ip, a.scope, a.target)
		}
		g.state.Failures[key] = f
		g.dirty = true
	}
	if newBan {
		_ = g.flushLocked()
	} // Make effective bans durable before acknowledging the failed attempt.
}

func (g *Manager) Stream(rawIP string) (func(), bool) {
	ip, err := CanonicalIP(rawIP)
	if err != nil {
		return nil, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, false
	}
	if g.streams >= 256 || g.streamIPs[ip] >= 12 {
		g.eventLocked("connection_limit", ip, "stream", "")
		return nil, false
	}
	g.streams++
	g.streamIPs[ip]++
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			g.streams--
			g.streamIPs[ip]--
			if g.streamIPs[ip] == 0 {
				delete(g.streamIPs, ip)
			}
			g.mu.Unlock()
		})
	}, true
}
func (g *Manager) Note(rawIP, scope, kind string) {
	ip, err := CanonicalIP(rawIP)
	if err != nil || !validScope(scope) {
		return
	}
	switch kind {
	case "origin_denied", "invalid_message", "message_limit", "request_rejected":
	default:
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.eventLocked(kind, ip, scope, "")
	if g.closed {
		return
	}
	if kind != "request_rejected" {
		g.abuseLocked(ip, scope)
	}
}
func (g *Manager) abuseLocked(ip, scope string) {
	key := "abuse:" + scope + ":" + ip
	f, exists := g.state.Failures[key]
	if !exists && len(g.state.Failures) >= maxKeys {
		return
	}
	now := g.now()
	if !now.Before(f.Reset) {
		f.Count = 0
		f.Reset = now.Add(2 * time.Minute)
	}
	f.Count++
	f.Expires = now.Add(10 * time.Minute)
	if f.Count >= 30 && len(g.state.Bans) < maxBans {
		f.Count = 0
		b := Ban{ID: newID(), Kind: "ip", IP: ip, Scope: scope, Reason: "repeated_abuse", Until: now.Add(5 * time.Minute)}
		g.state.Bans[b.ID] = b
		g.eventLocked("auto_ban", ip, scope, "")
		g.state.Failures[key] = f
		_ = g.flushLocked()
		return
	}
	g.state.Failures[key] = f
	g.dirty = true
}
func (g *Manager) AddBan(rawIP string, minutes int) (Ban, error) {
	ip, err := CanonicalIP(rawIP)
	if err != nil || minutes < 1 || minutes > 7*24*60 {
		return Ban{}, errors.New("invalid ban IP or duration")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweepLocked()
	if g.closed {
		return Ban{}, errors.New("security state is closed")
	}
	if len(g.state.Bans) >= maxBans {
		return Ban{}, errors.New("ban capacity reached")
	}
	b := Ban{ID: newID(), IP: ip, Kind: "ip", Scope: "all", Reason: "manual", Until: g.now().Add(time.Duration(minutes) * time.Minute)}
	g.state.Bans[b.ID] = b
	g.eventLocked("manual_ban", ip, "all", "")
	if err = g.flushLocked(); err != nil {
		return Ban{}, err
	}
	return b, nil
}
func (g *Manager) RemoveBan(id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return errors.New("security state is closed")
	}
	b, ok := g.state.Bans[id]
	if !ok {
		return errors.New("ban not found")
	}
	delete(g.state.Bans, id)
	if b.Kind == "ip" {
		delete(g.state.Failures, "ip:"+b.Scope+":"+b.IP)
	} else {
		delete(g.state.Failures, "target:"+b.Scope+":"+b.Target)
	}
	g.eventLocked("unban", b.IP, b.Scope, b.Target)
	return g.flushLocked()
}

type View struct {
	ClientIP        string            `json:"client_ip,omitempty"`
	PrivateClientIP bool              `json:"private_client_ip"`
	Bans            []Ban             `json:"bans"`
	Events          []Event           `json:"events"`
	ActiveBans      int               `json:"active_bans"`
	Connections     int               `json:"connections"`
	PersistenceOK   bool              `json:"persistence_ok"`
	Policies        map[string]Policy `json:"policies"`
	RetentionDays   int               `json:"retention_days"`
	EventLimit      int               `json:"event_limit"`
}

func (g *Manager) View() View {
	g.mu.Lock()
	defer g.mu.Unlock()
	v := View{Bans: []Ban{}, Events: []Event{}, PersistenceOK: !g.lastError, Connections: g.streams, Policies: g.opts.Policies, RetentionDays: 7, EventLimit: maxEvents}
	for _, b := range g.state.Bans {
		if g.now().Before(b.Until) {
			v.Bans = append(v.Bans, b)
		}
	}
	sort.Slice(v.Bans, func(i, j int) bool { return v.Bans[i].Until.After(v.Bans[j].Until) })
	v.ActiveBans = len(v.Bans)
	for _, e := range g.state.Events {
		if g.now().Sub(e.At) < retention {
			v.Events = append(v.Events, e)
		}
	}
	sort.Slice(v.Events, func(i, j int) bool { return v.Events[i].At.After(v.Events[j].At) })
	return v
}

// Recover is deliberately offline: the OS lock refuses recovery while the API owns its state.
func Recover(directory, ip, id string, all bool) (int, error) {
	lock, err := openLock(directory)
	if err != nil {
		return 0, err
	}
	defer releaseLock(lock)
	g := &Manager{opts: Options{Directory: directory, Now: time.Now}}
	if err = g.load(); err != nil {
		return 0, err
	}
	if !all && id == "" {
		ip, err = CanonicalIP(ip)
		if err != nil {
			return 0, err
		}
	}
	count := 0
	for key, b := range g.state.Bans {
		if all || key == id || (ip != "" && b.IP == ip) {
			delete(g.state.Bans, key)
			count++
		}
	}
	if all {
		g.state.Failures = map[string]failure{}
	} else if ip != "" {
		for key := range g.state.Failures {
			if strings.HasSuffix(key, ":"+ip) {
				delete(g.state.Failures, key)
			}
		}
	}
	if count == 0 && !all {
		return 0, fmt.Errorf("no matching ban")
	}
	return count, g.flushLocked()
}
