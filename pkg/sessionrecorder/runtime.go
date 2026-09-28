package sessionrecorder

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// LockFileName is created inside work_dir while the consumer runs.
const LockFileName = "session-recorder.lock"

// Snapshot is what the relay hot path reads: one atomic load, no locks.
type Snapshot struct {
	Settings Settings
	Filter   *Filter
	Producer *Producer
}

type consumerRunner struct {
	cancel   context.CancelFunc
	done     chan struct{}
	consumer atomic.Pointer[Consumer]
	store    atomic.Pointer[R2Store]
	state    atomic.Pointer[string]
	creds    atomic.Pointer[CredentialState]
}

type manager struct {
	mu          sync.Mutex
	raw         string
	applied     bool
	settings    Settings
	consumerKey string
	producerKey string
	runner      *consumerRunner
	snapshot    atomic.Pointer[Snapshot]
	applyErr    atomic.Pointer[string]
	started     bool
	stop        chan struct{}
}

var global = &manager{}

// Current returns the active snapshot, or nil while recording is off.
func Current() *Snapshot {
	return global.snapshot.Load()
}

// Start begins watching the SessionRecorderSetting option. It is safe to
// call when the feature is disabled: nothing else starts until enabled.
func Start() {
	global.mu.Lock()
	if global.started {
		global.mu.Unlock()
		return
	}
	global.started = true
	global.stop = make(chan struct{})
	stop := global.stop
	global.mu.Unlock()
	global.reloadFromOptions()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				common.SysError(fmt.Sprintf("session recorder watcher panic: %v", r))
			}
		}()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				global.reloadFromOptions()
			}
		}
	}()
}

// Stop shuts everything down within timeout.
func Stop(timeout time.Duration) {
	global.mu.Lock()
	defer global.mu.Unlock()
	if !global.started {
		return
	}
	close(global.stop)
	global.started = false
	global.stopLocked(timeout)
}

func (m *manager) stopLocked(timeout time.Duration) {
	if snap := m.snapshot.Swap(nil); snap != nil && snap.Producer != nil {
		snap.Producer.Stop(timeout)
	}
	if m.runner != nil {
		m.runner.shutdown(timeout)
		m.runner = nil
	}
	m.consumerKey, m.producerKey = "", ""
	m.applied = false
}

func (m *manager) reloadFromOptions() {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[OptionKey]
	common.OptionMapRWMutex.RUnlock()
	if err := Apply(raw); err != nil {
		msg := err.Error()
		m.applyErr.Store(&msg)
	}
}

func keyOf(v any) string {
	raw, _ := common.Marshal(v)
	return string(raw)
}

// Apply activates a settings JSON string. Unchanged components keep
// running; the filter swaps atomically.
func Apply(raw string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("session recorder apply panic: %v", r)
		}
	}()
	m := global
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started {
		return nil
	}
	if m.applied && raw == m.raw {
		return nil
	}
	s, err := ParseSettings(raw)
	if err == nil {
		err = s.Validate()
	}
	if err != nil {
		// Keep whatever is running; a bad option must not stop new-api.
		m.raw, m.applied = raw, true
		return err
	}
	m.raw, m.applied, m.settings = raw, true, s
	m.applyErr.Store(nil)
	if !s.Enabled {
		m.stopLocked(10 * time.Second)
		m.raw, m.applied = raw, true
		return nil
	}
	filter, _ := CompileFilter(s.Filter)

	// Producer.
	producerKey := ""
	if s.ProducerEnabled {
		producerKey = keyOf([]any{s.SpoolRoot, s.QueueSize, s.EffectiveStatsPath()})
	}
	old := m.snapshot.Load()
	var producer *Producer
	if old != nil && producerKey != "" && producerKey == m.producerKey {
		producer = old.Producer
	}
	if producerKey != "" && producer == nil {
		producer = NewProducer(s)
		producer.Start()
	}
	m.snapshot.Store(&Snapshot{Settings: s, Filter: filter, Producer: producer})
	if old != nil && old.Producer != nil && old.Producer != producer {
		go old.Producer.Stop(30 * time.Second)
	}
	m.producerKey = producerKey

	// Consumer.
	consumerKey := ""
	if s.ConsumerEnabled {
		consumerKey = keyOf([]any{s.ToConfig(), s.ScanIntervalSeconds, s.CredentialsDir, s.InstallationID})
	}
	if consumerKey != m.consumerKey {
		if m.runner != nil {
			m.runner.shutdown(30 * time.Second)
			m.runner = nil
		}
		if consumerKey != "" {
			m.runner = startConsumer(s)
		}
		m.consumerKey = consumerKey
	}
	return nil
}

func (r *consumerRunner) setState(s string) { r.state.Store(&s) }

func (r *consumerRunner) shutdown(timeout time.Duration) {
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(timeout):
	}
}

func startConsumer(s Settings) *consumerRunner {
	ctx, cancel := context.WithCancel(context.Background())
	r := &consumerRunner{cancel: cancel, done: make(chan struct{})}
	r.setState("starting")
	go func() {
		defer close(r.done)
		cfg := s.ToConfig()
		if err := EnsureDirectories(cfg); err != nil {
			r.setState("filesystem_error")
			common.SysError("session recorder: cannot create work_dir: " + err.Error())
			return
		}
		lock, err := AcquireProcessLock(filepath.Join(cfg.WorkDir, LockFileName))
		if err != nil {
			r.setState("lock_held")
			common.SysError("session recorder: " + err.Error())
			return
		}
		defer lock.Release()
		interval := time.Duration(max(5, s.ScanIntervalSeconds)) * time.Second
		for {
			r.cycle(ctx, s, cfg)
			select {
			case <-ctx.Done():
				r.setState("stopped")
				return
			case <-time.After(interval):
			}
		}
	}()
	return r
}

// cycle runs one consumer cycle; panics are contained here.
func (r *consumerRunner) cycle(ctx context.Context, s Settings, cfg Config) {
	defer func() {
		if rec := recover(); rec != nil {
			r.setState("panic_recovered")
			common.SysError(fmt.Sprintf("session recorder consumer panic: %v", rec))
		}
	}()
	creds, err := LoadCredentials(s.CredentialsDir)
	state := creds.State()
	if err != nil {
		state = CredentialInvalid
	}
	r.creds.Store(&state)
	consumer := r.consumer.Load()
	if consumer == nil {
		var store Store
		if cfg.UploadEnabled {
			if state != CredentialConfigured {
				r.setState("credentials_" + string(state))
				return
			}
			r2, err := NewR2Store(cfg, creds)
			if err != nil {
				r.setState("r2_config_error")
				return
			}
			r.store.Store(r2)
			store = r2
		}
		consumer = NewConsumer(cfg, store)
		consumer.SetLogger(func(format string, args ...any) { common.SysLog(fmt.Sprintf(format, args...)) })
		if cfg.QueueGuard.StatsPath == "" {
			consumer.SetQueueStats(func() (ProducerStats, bool) {
				snap := Current()
				if snap == nil || snap.Producer == nil {
					return ProducerStats{}, false
				}
				return snap.Producer.Stats(), true
			})
		}
		r.consumer.Store(consumer)
	}
	result := consumer.RunCycle(ctx)
	r.setState(result.State)
}

// StatusResponse is returned by the admin status API. It never includes
// credential values, only their state.
type StatusResponse struct {
	Enabled         bool              `json:"enabled"`
	ProducerRunning bool              `json:"producer_running"`
	ConsumerRunning bool              `json:"consumer_running"`
	ConsumerState   string            `json:"consumer_state"`
	CredentialState CredentialState   `json:"credential_state"`
	SettingsError   string            `json:"settings_error,omitempty"`
	Producer        *ProducerCounters `json:"producer,omitempty"`
	Report          *StatusReport     `json:"report,omitempty"`
	LastCycle       *CycleStatus      `json:"last_cycle,omitempty"`
	LastError       string            `json:"last_error,omitempty"`
	LastUploadMbps  float64           `json:"last_upload_mbps"`
	RecentIssues    []ScanIssue       `json:"recent_issues"`
}

type CycleStatus struct {
	At           string `json:"at"`
	State        string `json:"state"`
	BatchID      string `json:"batch_id,omitempty"`
	ScannedFiles int    `json:"scanned_files"`
	OrphanMedia  int    `json:"orphan_media"`
	Issues       int    `json:"issues"`
}

// Status collects the current runtime status.
func Status(settings Settings) StatusResponse {
	resp := StatusResponse{Enabled: settings.Enabled, RecentIssues: []ScanIssue{}}
	if e := global.applyErr.Load(); e != nil {
		resp.SettingsError = *e
	}
	creds, err := LoadCredentials(settings.CredentialsDir)
	resp.CredentialState = creds.State()
	if err != nil {
		resp.CredentialState = CredentialInvalid
	}
	if snap := Current(); snap != nil && snap.Producer != nil {
		counters := snap.Producer.Counters()
		resp.ProducerRunning = true
		resp.Producer = &counters
	}
	global.mu.Lock()
	runner := global.runner
	global.mu.Unlock()
	if runner == nil {
		resp.ConsumerState = "stopped"
		return resp
	}
	resp.ConsumerRunning = true
	if st := runner.state.Load(); st != nil {
		resp.ConsumerState = *st
	}
	if store := runner.store.Load(); store != nil {
		resp.LastUploadMbps = store.LastUploadMbps()
	}
	if consumer := runner.consumer.Load(); consumer != nil {
		report, result, lastErr, at := consumer.Status(resp.CredentialState)
		resp.Report = &report
		resp.LastError = lastErr
		if !at.IsZero() {
			resp.LastCycle = &CycleStatus{
				At: at.UTC().Format(time.RFC3339), State: result.State, BatchID: result.BatchID,
				ScannedFiles: result.ScannedFiles, OrphanMedia: result.OrphanMedia, Issues: result.Issues,
			}
		}
		resp.RecentIssues = consumer.RecentIssues(50)
	}
	return resp
}

// RunCheck performs the R2 connectivity probe with the given settings.
func RunCheck(ctx context.Context, settings Settings) (CredentialState, error) {
	creds, err := LoadCredentials(settings.CredentialsDir)
	state := creds.State()
	if err != nil {
		return CredentialInvalid, errors.New("credentials unreadable")
	}
	if state != CredentialConfigured {
		return state, fmt.Errorf("credentials %s", state)
	}
	cfg := settings.ToConfig()
	if cfg.InstallationID == "" {
		return state, errors.New("installation_id: required")
	}
	store, err := NewR2Store(cfg, creds)
	if err != nil {
		return state, err
	}
	return state, Check(ctx, cfg, store)
}
