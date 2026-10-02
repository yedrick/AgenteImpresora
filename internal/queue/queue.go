package queue

import (
	"encoding/json"
	"errors"
	"fmt"
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

const (
	// queueCapacity es cuantos trabajos caben esperando. Al llenarse se
	// rechaza con error en vez de bloquear: antes el envio al canal no tenia
	// salida y, con una impresora caida, los handlers HTTP se quedaban
	// colgados hasta el WriteTimeout.
	queueCapacity = 256

	// maxHistory acota el historial. El mapa no se purgaba nunca y
	// jobs.json se reescribia entero en cada cambio de estado, asi que el
	// coste crecia sin limite.
	maxHistory = 500

	// persistInterval agrupa las escrituras a disco.
	persistInterval = 2 * time.Second
)

// ErrQueueFull indica que la cola no admite mas trabajos ahora mismo.
var ErrQueueFull = errors.New("la cola de impresion esta llena")

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
	stopOnce   sync.Once
	mu         sync.RWMutex
	workers    int
	maxRetries int
	wg         sync.WaitGroup
	persistDir string
	locksMu    sync.Mutex
	locks      map[string]*sync.Mutex

	dirty   chan struct{}
	flushWG sync.WaitGroup

	seqMu sync.Mutex
	lastT time.Time
	seq   int
}

func NewManager(pm *printers.Manager, logger *logs.Logger, workers, maxRetries int) *Manager {
	if workers <= 0 {
		workers = 1
	}
	if maxRetries <= 0 {
		maxRetries = 1
	}
	return &Manager{
		printers:   pm,
		logger:     logger,
		jobs:       map[string]*Job{},
		queue:      make(chan *Job, queueCapacity),
		stop:       make(chan struct{}),
		dirty:      make(chan struct{}, 1),
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
	m.flushWG.Add(1)
	go m.flusher()
}

// Stop espera a que los workers terminen ANTES de guardar. Antes se
// persistia primero, asi que los ultimos cambios de estado se perdian.
func (m *Manager) Stop() {
	m.stopOnce.Do(func() {
		close(m.stop)
		m.wg.Wait()
		m.flushWG.Wait()
		m.persistToDisk()
	})
}

// nextID garantiza identificadores unicos. El formato de marca de tiempo tiene
// resolucion limitada, asi que dos peticiones simultaneas podian generar el
// mismo ID y pisarse en el mapa.
func (m *Manager) nextID(now time.Time) string {
	m.seqMu.Lock()
	defer m.seqMu.Unlock()
	if now.Equal(m.lastT) || now.Before(m.lastT) {
		m.seq++
		now = m.lastT.Add(time.Duration(m.seq) * time.Nanosecond)
	} else {
		m.lastT, m.seq = now, 0
	}
	return now.Format("20060102150405.000000000")
}

func (m *Manager) EnqueueMany(printerNames []string, payload []byte) ([]*Job, error) {
	now := time.Now().UTC()
	jobs := make([]*Job, 0, len(printerNames))
	for _, printer := range printerNames {
		if strings.TrimSpace(printer) == "" {
			continue
		}
		jobs = append(jobs, &Job{
			ID:        m.nextID(now),
			Printer:   printer,
			Status:    Pending,
			CreatedAt: now,
			UpdatedAt: now,
			Payload:   append([]byte(nil), payload...),
		})
	}
	if len(jobs) == 0 {
		return nil, fmt.Errorf("no hay impresoras de destino")
	}

	// Las copias para la respuesta se hacen aqui, con el lock tomado y antes
	// de que ningun worker pueda ver los trabajos: clonarlos despues del
	// envio al canal era una carrera de datos contra update().
	m.mu.Lock()
	out := make([]*Job, 0, len(jobs))
	for _, job := range jobs {
		m.jobs[job.ID] = job
		out = append(out, cloneJob(job))
	}
	m.pruneLocked()
	m.mu.Unlock()

	for _, job := range jobs {
		select {
		case m.queue <- job:
		case <-m.stop:
			return nil, fmt.Errorf("el agente se esta deteniendo")
		default:
			m.mu.Lock()
			delete(m.jobs, job.ID)
			m.mu.Unlock()
			if m.logger != nil {
				m.logger.Error("print_rejected", map[string]any{"printer": job.Printer, "reason": "cola llena"})
			}
			return nil, ErrQueueFull
		}
		if m.logger != nil {
			m.logger.Info("print_enqueued", map[string]any{"job_id": job.ID, "printer": job.Printer, "bytes": len(payload)})
		}
	}
	m.markDirty()
	return out, nil
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

	m.mu.RLock()
	payload := job.Payload
	m.mu.RUnlock()

	for attempt := 1; attempt <= m.maxRetries; attempt++ {
		m.update(job, Printing, attempt, "")
		err := m.printers.Print(job.Printer, payload)
		if err == nil {
			m.update(job, Completed, attempt, "")
			m.finalize(job)
			return
		}
		if m.logger != nil {
			m.logger.Error("print_attempt_failed", map[string]any{
				"job_id": job.ID, "printer": job.Printer, "attempt": attempt, "error": err.Error(),
			})
		}
		if attempt < m.maxRetries {
			select {
			case <-time.After(time.Duration(1<<uint(attempt-1)) * time.Second):
			case <-m.stop:
				m.update(job, Failed, attempt, "interrumpido al detener el agente")
				m.finalize(job)
				return
			}
			continue
		}
		m.update(job, Failed, attempt, err.Error())
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
	job.Status = status
	job.Attempts = attempts
	job.Error = errMsg
	job.UpdatedAt = time.Now().UTC()
	m.mu.Unlock()
	// El log se escribe fuera del lock: es I/O y no tiene por que bloquear a
	// quien consulte /api/status.
	if m.logger != nil {
		m.logger.Info("print_status", map[string]any{"job_id": job.ID, "status": status, "attempts": attempts})
	}
	m.markDirty()
}

func (m *Manager) finalize(job *Job) {
	m.mu.Lock()
	job.Payload = nil // el historial no necesita el contenido
	m.pruneLocked()
	m.mu.Unlock()
	m.markDirty()
}

// pruneLocked recorta el historial a los maxHistory trabajos mas recientes,
// sin tocar los que siguen pendientes o imprimiendose.
func (m *Manager) pruneLocked() {
	if len(m.jobs) <= maxHistory {
		return
	}
	done := make([]*Job, 0, len(m.jobs))
	for _, job := range m.jobs {
		if job.Status == Completed || job.Status == Failed {
			done = append(done, job)
		}
	}
	sort.Slice(done, func(i, j int) bool { return done[i].CreatedAt.Before(done[j].CreatedAt) })
	for i := 0; i < len(done) && len(m.jobs) > maxHistory; i++ {
		delete(m.jobs, done[i].ID)
	}
}

// markDirty pide un guardado. Las escrituras se agrupan para no reescribir el
// archivo entero en cada cambio de estado.
func (m *Manager) markDirty() {
	select {
	case m.dirty <- struct{}{}:
	default:
	}
}

func (m *Manager) flusher() {
	defer m.flushWG.Done()
	ticker := time.NewTicker(persistInterval)
	defer ticker.Stop()
	pending := false
	for {
		select {
		case <-m.stop:
			return
		case <-m.dirty:
			pending = true
		case <-ticker.C:
			if pending {
				m.persistToDisk()
				pending = false
			}
		}
	}
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

	if err := os.MkdirAll(m.persistDir, 0o755); err != nil {
		return
	}
	path := filepath.Join(m.persistDir, "jobs.json")
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	// 0600: el archivo lleva el contenido de los tickets pendientes.
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
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
	var resume []*Job
	m.mu.Lock()
	for i := range saved {
		job := saved[i]
		m.jobs[job.ID] = &job
		switch job.Status {
		case Pending:
			if len(job.Payload) == 0 {
				// Guardado por una version que no persistia el payload: no se
				// puede reimprimir, asi que se marca fallido en vez de fingir
				// que se imprimio.
				job.Status = Failed
				job.Error = "sin contenido guardado: no se pudo reanudar"
				job.UpdatedAt = time.Now().UTC()
				continue
			}
			resume = append(resume, &job)
		case Printing:
			job.Status = Failed
			job.Error = "interrumpido por reinicio del agente"
			job.UpdatedAt = time.Now().UTC()
			job.Payload = nil
			if m.logger != nil {
				m.logger.Error("print_interrupted", map[string]any{"job_id": job.ID, "printer": job.Printer})
			}
		}
	}
	m.pruneLocked()
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
