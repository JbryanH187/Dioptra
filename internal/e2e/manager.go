package e2e

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RunStore persiste corridas como documento JSON.
type RunStore interface {
	SaveE2ERun(ctx context.Context, id, project, title, status string, doc []byte) error
	LoadE2ERun(ctx context.Context, id string) ([]byte, error)
}

// Publisher publica (o actualiza) el reporte de una corrida en Dioptra.
type Publisher interface {
	PublishE2E(ctx context.Context, run *Run, markdown string) (reportID, url string, err error)
	// CheckedItems devuelve las casillas marcadas del reporte (texto → true).
	CheckedItems(ctx context.Context, reportID string) map[string]bool
}

// Result es lo que devuelven las herramientas E2E al agente.
type Result struct {
	Run        string    `json:"run"`
	Status     string    `json:"status"`
	ReportID   string    `json:"report_id,omitempty"`
	URL        string    `json:"url,omitempty"`
	Case       int       `json:"case,omitempty"`
	Cases      int       `json:"cases"`
	Steps      int       `json:"steps"`
	Services   int       `json:"services"`
	Findings   int       `json:"findings"`
	Added      int       `json:"added,omitempty"`
	Redactions int       `json:"redactions,omitempty"`
	HAR        *HARStats `json:"har,omitempty"`
	Message    string    `json:"message"`
}

// Manager coordina las corridas: enmascara, persiste y vuelve a publicar el reporte.
type Manager struct {
	Store RunStore
	Pub   Publisher
	Now   func() time.Time
	mu    sync.Mutex
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func newRunID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return "e2e_" + hex.EncodeToString(b[:])
}

func (m *Manager) load(ctx context.Context, id string) (*Run, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("falta run (el id que devolvió e2e_start)")
	}
	doc, err := m.Store.LoadE2ERun(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("no encontré la corrida %s: %w", id, err)
	}
	var r Run
	if err := json.Unmarshal(doc, &r); err != nil {
		return nil, fmt.Errorf("corrida %s ilegible: %w", id, err)
	}
	return &r, nil
}

// save persiste y publica; devuelve el resultado para el agente.
func (m *Manager) save(ctx context.Context, r *Run, caseN int, msg string) (*Result, error) {
	done := map[string]bool{}
	if r.ReportID != "" && m.Pub != nil {
		done = m.Pub.CheckedItems(ctx, r.ReportID)
	}
	link := ""
	if m.Pub != nil {
		id, u, err := m.Pub.PublishE2E(ctx, r, Markdown(r, done))
		if err != nil {
			return nil, fmt.Errorf("no pude publicar el reporte E2E: %w", err)
		}
		r.ReportID, link = id, u
	}
	doc, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	if err := m.Store.SaveE2ERun(ctx, r.ID, r.Project, r.Title, r.Status, doc); err != nil {
		return nil, err
	}
	return &Result{
		Run: r.ID, Status: r.Status, ReportID: r.ReportID, URL: link, Case: caseN,
		Cases: len(r.Cases), Steps: r.TotalSteps(), Services: len(r.Services), Findings: len(r.Findings),
		Redactions: r.Redactions, Message: msg,
	}, nil
}

// Start abre una corrida y publica el reporte vacío (estado "grabando").
func (m *Manager) Start(ctx context.Context, project, title, environment, agent string) (*Result, error) {
	if strings.TrimSpace(title) == "" {
		return nil, errors.New("falta title (p. ej. \"Abono · QA\")")
	}
	if strings.TrimSpace(project) == "" {
		return nil, errors.New("falta project (nombre de la carpeta raíz del repo)")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r := NewRun(newRunID(), project, title, environment, agent, m.now())
	return m.save(ctx, r, 0, "Observador E2E encendido. Registra servicios, casos y pasos con e2e_record; al terminar, e2e_finish.")
}

// Record aplica un evento (caso, paso, servicio, hallazgo o nota).
func (m *Manager) Record(ctx context.Context, runID string, ev Event) (*Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.load(ctx, runID)
	if err != nil {
		return nil, err
	}
	red := &Redactor{}
	ev.Request, ev.Response = red.Body(ev.Request), red.Body(ev.Response)
	ev.Log = Truncate(red.Text(ev.Log), MaxBodyBytes)
	ev.URL, ev.Detail, ev.Title = red.Text(ev.URL), red.Text(ev.Detail), red.Text(ev.Title)
	ev.Issue, ev.Suggestion, ev.Result = red.Text(ev.Issue), red.Text(ev.Suggestion), red.Text(ev.Result)
	if ev.Kind == "step" && ev.Service != "" {
		ev.Service = friendlyService(r, ev.Service)
	}
	caseN, err := r.Apply(ev, m.now())
	if err != nil {
		return nil, err
	}
	r.Redactions += red.Count
	msg := "Registrado."
	if red.Count > 0 {
		msg = fmt.Sprintf("Registrado (%d datos sensibles ocultos).", red.Count)
	}
	return m.save(ctx, r, caseN, msg)
}

// IngestHAR convierte un HAR exportado del navegador en pasos del caso indicado.
func (m *Manager) IngestHAR(ctx context.Context, runID string, caseN int, path string) (*Result, error) {
	path = strings.TrimSpace(path)
	if !strings.EqualFold(filepath.Ext(path), ".har") {
		return nil, errors.New("path debe ser un archivo .har (DevTools › Network › Save all as HAR)")
	}
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path[1:], string(filepath.Separator)))
		}
	}
	fh, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("no pude abrir el HAR: %w", err)
	}
	defer fh.Close()

	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.load(ctx, runID)
	if err != nil {
		return nil, err
	}
	if r.Status == StatusFinished {
		return nil, ErrFinished
	}
	red := &Redactor{}
	steps, services, stats, err := ParseHAR(fh, red)
	if err != nil {
		return nil, err
	}
	for _, s := range services {
		if friendlyService(r, s.Name) == s.Name && !hasService(r, s.Name) {
			r.Services = append(r.Services, s)
		}
	}
	for i := range steps {
		steps[i].Service = friendlyService(r, steps[i].Service)
		steps[i].From = friendlyService(r, steps[i].From)
	}
	n, err := r.AddSteps(caseN, steps, m.now())
	if err != nil {
		return nil, err
	}
	r.Redactions += red.Count
	r.Updated = m.now().UTC().Format(time.RFC3339)
	res, err := m.save(ctx, r, n, fmt.Sprintf("HAR: %d requests → %d pasos en el caso %d (%d estáticos omitidos, %d datos sensibles ocultos).",
		stats.Entries, stats.Steps, n, stats.Skipped, red.Count))
	if err != nil {
		return nil, err
	}
	res.Added, res.HAR = len(steps), &stats
	return res, nil
}

// Finish cierra la corrida con un resumen.
func (m *Manager) Finish(ctx context.Context, runID, summary string) (*Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.load(ctx, runID)
	if err != nil {
		return nil, err
	}
	if r.Status == StatusFinished {
		return nil, ErrFinished
	}
	red := &Redactor{}
	r.Summary = red.Text(strings.TrimSpace(summary))
	r.Redactions += red.Count
	r.Status = StatusFinished
	r.Updated = m.now().UTC().Format(time.RFC3339)
	return m.save(ctx, r, 0, fmt.Sprintf("Corrida cerrada: %d casos, %d pasos, %d servicios, %d mejoras de logs.",
		len(r.Cases), r.TotalSteps(), len(r.Services), len(r.Findings)))
}

func hasService(r *Run, name string) bool {
	for _, s := range r.Services {
		if strings.EqualFold(s.Name, name) {
			return true
		}
	}
	return false
}

// friendlyService traduce un host (localhost:8081) al nombre que el agente registró
// para ese servicio (api-clientes), si su base_url apunta al mismo host.
func friendlyService(r *Run, hostOrName string) string {
	for _, s := range r.Services {
		if strings.EqualFold(s.Name, hostOrName) {
			return s.Name
		}
		if s.BaseURL == "" {
			continue
		}
		if u, err := url.Parse(s.BaseURL); err == nil && strings.EqualFold(u.Host, hostOrName) {
			return s.Name
		}
	}
	return hostOrName
}
