package queue

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// impresoraFalsa acepta cualquier trabajo sin hacer nada.
type impresoraFalsa struct{}

func (impresoraFalsa) Print(string, []byte) error { return nil }

func chdirTemp(t *testing.T) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// persistToDisk guardaba clones sin payload, asi que al reiniciar los
// trabajos pendientes se reencolaban vacios, la impresora no recibia nada y
// la cola los marcaba Completed.
func TestElPayloadDeLosPendientesSePersiste(t *testing.T) {
	chdirTemp(t)
	m := New(Options{Printers: impresoraFalsa{}, Workers: 1, MaxRetries: 1})
	jobs, err := m.EnqueueMany([]string{"POS1"}, []byte("CONTENIDO DEL TICKET"))
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("esperaba 1 trabajo, obtuve %d", len(jobs))
	}

	m.persistToDisk()
	raw, err := os.ReadFile(filepath.Join("storage", "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved []Job
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || string(saved[0].Payload) != "CONTENIDO DEL TICKET" {
		t.Fatalf("el payload no se persistio: %+v", saved)
	}

	// Un reinicio debe recuperarlo con su contenido intacto.
	m2 := New(Options{Printers: impresoraFalsa{}, Workers: 1, MaxRetries: 1})
	m2.loadFromDisk()
	select {
	case job := <-m2.queue:
		if string(job.Payload) != "CONTENIDO DEL TICKET" {
			t.Fatalf("se reencolo sin contenido: %q", job.Payload)
		}
	default:
		t.Fatal("el trabajo pendiente no se reencolo")
	}
}

// Los trabajos terminados sueltan el payload para no engordar el archivo.
func TestLosTrabajosTerminadosNoGuardanElPayload(t *testing.T) {
	chdirTemp(t)
	m := New(Options{Printers: impresoraFalsa{}, Workers: 1, MaxRetries: 1})
	jobs, err := m.EnqueueMany([]string{"POS1"}, []byte("secreto"))
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.jobs[jobs[0].ID].Status = Completed
	m.mu.Unlock()
	m.persistToDisk()

	raw, readErr := os.ReadFile(filepath.Join("storage", "jobs.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	var saved []Job
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved[0].Payload) != 0 {
		t.Fatalf("un trabajo terminado no deberia guardar el payload: %q", saved[0].Payload)
	}
}

// Un trabajo guardado por una version antigua, sin payload, no se puede
// reimprimir: debe quedar como fallido en vez de fingir que se imprimio.
func TestUnPendienteSinPayloadQuedaComoFallido(t *testing.T) {
	chdirTemp(t)
	os.MkdirAll("storage", 0o755)
	antiguo := []Job{{
		ID: "20260101000000.000000000", Printer: "POS1", Status: Pending,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}}
	b, _ := json.Marshal(antiguo)
	os.WriteFile(filepath.Join("storage", "jobs.json"), b, 0o644)

	m := New(Options{Printers: impresoraFalsa{}, Workers: 1, MaxRetries: 1})
	m.loadFromDisk()
	select {
	case job := <-m.queue:
		t.Fatalf("no deberia reencolarse un trabajo sin contenido: %+v", job)
	default:
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if got := m.jobs[antiguo[0].ID].Status; got != Failed {
		t.Fatalf("esperaba Failed, obtuve %s", got)
	}
}

// Al llenarse la cola hay que rechazar, no bloquear al handler HTTP.
func TestLaColaLlenaRechazaEnVezDeBloquear(t *testing.T) {
	chdirTemp(t)
	m := New(Options{Printers: impresoraFalsa{}, Workers: 1, MaxRetries: 1}) // sin Start: nadie consume
	for i := 0; i < queueCapacity; i++ {
		if _, err := m.EnqueueMany([]string{"POS1"}, []byte("x")); err != nil {
			t.Fatalf("trabajo %d: %v", i, err)
		}
	}
	hecho := make(chan error, 1)
	go func() {
		_, err := m.EnqueueMany([]string{"POS1"}, []byte("uno de mas"))
		hecho <- err
	}()
	select {
	case err := <-hecho:
		if !errors.Is(err, ErrQueueFull) {
			t.Fatalf("esperaba ErrQueueFull, obtuve %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("EnqueueMany se quedo bloqueado con la cola llena")
	}
}

// Dos peticiones simultaneas podian generar el mismo ID y pisarse en el mapa.
func TestLosIdentificadoresNoColisionan(t *testing.T) {
	chdirTemp(t)
	m := New(Options{Printers: impresoraFalsa{}, Workers: 1, MaxRetries: 1})
	vistos := map[string]bool{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			jobs, err := m.EnqueueMany([]string{"POS1"}, []byte("x"))
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, j := range jobs {
				if vistos[j.ID] {
					t.Errorf("ID repetido: %s", j.ID)
				}
				vistos[j.ID] = true
			}
		}()
	}
	wg.Wait()
	if len(vistos) != 100 {
		t.Fatalf("esperaba 100 identificadores unicos, obtuve %d", len(vistos))
	}
}

// El historial no puede crecer sin limite.
func TestElHistorialSeRecorta(t *testing.T) {
	chdirTemp(t)
	m := New(Options{Printers: impresoraFalsa{}, Workers: 1, MaxRetries: 1})
	for i := 0; i < maxHistory+50; i++ {
		id := m.nextID(time.Now().UTC())
		m.mu.Lock()
		m.jobs[id] = &Job{ID: id, Printer: "POS1", Status: Completed, CreatedAt: time.Now().UTC()}
		m.pruneLocked()
		m.mu.Unlock()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.jobs) > maxHistory {
		t.Fatalf("el historial crecio hasta %d, el limite es %d", len(m.jobs), maxHistory)
	}
}
