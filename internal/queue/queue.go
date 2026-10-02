package queue

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"collatech-agent/internal/logs"
	"collatech-agent/internal/printers"
)

type Status string

const (
	Pending   Status = "Pending"
	Printing  Status = "Printing"
	Completed Status = "Completed"
	Failed    Status = "Failed"
)

type Job struct {
	ID        string    `json:"id"`
	Printer   string    `json:"printer"`
	Status    Status    `json:"status"`
	Attempts  int       `json:"attempts"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Payload   []byte    `json:"payload,omitempty"`
}

type Manager struct {
	printers   *printers.Manager
	logger     *logs.Logger
	jobs       map[string]*Job
	queue      chan *Job
	stop       chan struct{}
	mu         sync.RWMutex
	workers    int
	maxRetries int
	wg         sync.WaitGroup
	persistDir string
	locksMu    sync.Mutex
	locks      map[string]*sync.Mutex
}

func NewManager(pm *printers.Manager, logger *logs.Logger, workers, maxRetries int) *Manager {
	if workers <= 0 {
		workers = 2
	}
	if maxRetries <= 0 {
		maxRetries = 2
	}
	return &Manager{
		printers:   pm,
		logger:     logger,
		jobs:       map[string]*Job{},
		queue:      make(chan *Job, 256),
		stop:       make(chan struct{}),
		workers:    workers,
		maxRetries: maxRetries,
		persistDir: "storage",
		locks:      map[string]*sync.Mutex{},
	}
}

func (m *Manager) Start() {
	m.loadFromDisk()
	for i := 0; i < m.workers; i++ {
		m.wg.Add(1)
		go m.worker()
	}
}

func (m *Manager) Stop() {
	close(m.stop)
	m.persistToDisk()
	m.wg.Wait()
}

func (m *Manager) EnqueueMany(printers []string, payload []byte) []*Job {
	now := time.Now().UTC()
	jobs := make([]*Job, 0, len(printers))
	for i, printer := range printers {
		if strings.TrimSpace(printer) == "" {
			continue
		}
		jobTime := now.Add(time.Duration(i) * time.Nanosecond)
		jobs = append(jobs, &Job{
			ID:        jobTime.Format("20060102150405.000000000"),
			Printer:   printer,
			Status:    Pending,
			CreatedAt: jobTime,
			UpdatedAt: jobTime,
			Payload:   append([]byte(nil), payload...),
		})
	}
	if len(jobs) == 0 {
		return nil
	}
	m.mu.Lock()
	for _, job := range jobs {
		m.jobs[job.ID] = job
	}
	m.mu.Unlock()
	m.persistToDisk()
	for _, job := range jobs {
		m.queue <- job
		if m.logger != nil {
			m.logger.Info("print_enqueued", map[string]any{"job_id": job.ID, "printer": job.Printer, "bytes": len(payload)})
		}
	}
	out := make([]*Job, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, cloneJob(job))
	}
	return out
}

func (m *Manager) List() []Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Job, 0, len(m.jobs))
	for _, job := range m.jobs {
		out = append(out, *cloneJob(job))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (m *Manager) worker() {
	defer m.wg.Done()
	for {
		select {
		case <-m.stop:
			return
		case job := <-m.queue:
			m.process(job)
		}
	}
}

func (m *Manager) process(job *Job) {
	lock := m.printerLock(job.Printer)
	lock.Lock()
	defer lock.Unlock()
	for attempt := 1; attempt <= m.maxRetries; attempt++ {
		m.update(job, Printing, attempt, "")
		err := m.printers.Print(job.Printer, job.Payload)
		if err == nil {
			m.update(job, Completed, attempt, "")
			m.finalize(job)
			return
		}
		m.update(job, Failed, attempt, err.Error())
		if m.logger != nil {
			m.logger.Error("print_attempt_failed", map[string]any{
				"job_id": job.ID, "printer": job.Printer, "attempt": attempt, "error": err.Error(),
			})
		}
		if attempt < m.maxRetries {
			time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
		}
	}
	m.finalize(job)
}

func (m *Manager) printerLock(printer string) *sync.Mutex {
	key := strings.ToLower(strings.TrimSpace(printer))
	if key == "" {
		key = "__default__"
	}
	m.locksMu.Lock()
	defer m.locksMu.Unlock()
	lock, ok := m.locks[key]
	if !ok {
		lock = &sync.Mutex{}
		m.locks[key] = lock
	}
	return lock
}

func (m *Manager) update(job *Job, status Status, attempts int, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job.Status = status
	job.Attempts = attempts
	job.Error = errMsg
	job.UpdatedAt = time.Now().UTC()
	if m.logger != nil {
		m.logger.Info("print_status", map[string]any{"job_id": job.ID, "status": status, "attempts": attempts})
	}
}

func (m *Manager) finalize(job *Job) {
	// Drop payload to keep history file small
	m.mu.Lock()
	job.Payload = nil
	m.mu.Unlock()
	m.persistToDisk()
}

func (m *Manager) persistToDisk() {
	m.mu.RLock()
	out := make([]Job, 0, len(m.jobs))
	for _, job := range m.jobs {
		cp := *job
		// Solo los trabajos sin terminar conservan el payload: es lo unico que
		// permite reanudarlos tras un reinicio. Antes se persistian clones sin
		// payload, asi que al arrancar se reencolaban vacios y se marcaban
		// Completed sin haber impreso nada.
		if cp.Status != Pending && cp.Status != Printing {
			cp.Payload = nil
		}
		out = append(out, cp)
	}
	m.mu.RUnlock()
	if len(out) == 0 {
		return
	}
	if err := os.MkdirAll(m.persistDir, 0o755); err != nil {
		return
	}
	path := filepath.Join(m.persistDir, "jobs.json")
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

func (m *Manager) loadFromDisk() {
	path := filepath.Join(m.persistDir, "jobs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var saved []Job
	if err := json.Unmarshal(data, &saved); err != nil {
		return
	}
	resume := []*Job{}
	m.mu.Lock()
	for i := range saved {
		job := saved[i]
		m.jobs[job.ID] = &job
		if job.Status == Pending {
			if len(job.Payload) == 0 {
				// Trabajo guardado por una version anterior, que no persistia
				// el payload: no se puede reimprimir, se marca como fallido en
				// vez de fingir que se imprimio.
				job.Status = Failed
				job.Error = "sin contenido guardado: no se pudo reanudar"
				job.UpdatedAt = time.Now().UTC()
				m.jobs[job.ID] = &job
				continue
			}
			resume = append(resume, &job)
		} else if job.Status == Printing {
			job.Status = Failed
			job.Error = "interrumpido por reinicio del agente"
			job.UpdatedAt = time.Now().UTC()
			job.Payload = nil
			m.jobs[job.ID] = &job
			if m.logger != nil {
				m.logger.Error("print_interrupted", map[string]any{"job_id": job.ID, "printer": job.Printer})
			}
		}
	}
	m.mu.Unlock()
	for _, j := range resume {
		select {
		case m.queue <- j:
		default:
		}
	}
}

func cloneJob(job *Job) *Job {
	cp := *job
	cp.Payload = nil
	return &cp
}
