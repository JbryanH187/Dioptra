// Package e2e implementa el Observador E2E: un agente (cualquiera, vía MCP) documenta
// una prueba de punta a punta que hace una persona —pasos, requests/responses,
// servicios, logs, validaciones y calidad de logs— y Dioptra lo muestra como un
// reporte vivo organizado por caso de prueba.
package e2e

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Estados de una corrida.
const (
	StatusRecording = "grabando"
	StatusFinished  = "terminado"
)

// Run es una corrida de observación E2E (una sesión de pruebas).
type Run struct {
	ID          string    `json:"id"`
	Project     string    `json:"project"`
	Title       string    `json:"title"`
	Environment string    `json:"environment,omitempty"`
	Agent       string    `json:"agent,omitempty"`
	Status      string    `json:"status"`
	Started     string    `json:"started"`
	Updated     string    `json:"updated"`
	ReportID    string    `json:"report_id,omitempty"`
	Services    []Service `json:"services"`
	Cases       []Case    `json:"cases"`
	Findings    []Finding `json:"findings"`
	Notes       []string  `json:"notes,omitempty"`
	Summary     string    `json:"summary,omitempty"`
	Redactions  int       `json:"redactions"`
}

// Service es un componente que participa en el flujo.
type Service struct {
	Name        string `json:"name"`
	BaseURL     string `json:"base_url,omitempty"`
	Environment string `json:"environment,omitempty"`
	Port        string `json:"port,omitempty"`
	LogSource   string `json:"log_source,omitempty"`
}

// Case es un caso de prueba (prueba 1, 2, 3…).
type Case struct {
	N        int    `json:"n"`
	Title    string `json:"title"`
	Expected string `json:"expected,omitempty"`
	Result   string `json:"result,omitempty"`
	Steps    []Step `json:"steps"`
}

// Step es un paso observado dentro de un caso.
type Step struct {
	Seq        int    `json:"seq"`
	Time       string `json:"time"`
	Kind       string `json:"kind"` // request | query | validation | log | ui | note
	Title      string `json:"title,omitempty"`
	From       string `json:"from,omitempty"`    // quién llama (p. ej. frontend)
	Service    string `json:"service,omitempty"` // a quién llama
	Method     string `json:"method,omitempty"`
	URL        string `json:"url,omitempty"`
	Status     int    `json:"status,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Request    string `json:"request,omitempty"`
	Response   string `json:"response,omitempty"`
	Log        string `json:"log,omitempty"`
	Verdict    string `json:"verdict,omitempty"` // ok | warn | error | info
	Detail     string `json:"detail,omitempty"`
}

// Finding es una oportunidad de mejora en los logs (u otra observación de calidad).
type Finding struct {
	N          int    `json:"n"`
	Service    string `json:"service,omitempty"`
	Severity   string `json:"severity"` // info | warning | error
	Issue      string `json:"issue"`
	Where      string `json:"where,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

// Event es lo que registra el agente con `e2e_record`.
type Event struct {
	Kind string `json:"kind"` // case | step | service | finding | note

	Case     int    `json:"case,omitempty"`
	Title    string `json:"title,omitempty"`
	Expected string `json:"expected,omitempty"`
	Result   string `json:"result,omitempty"`

	StepKind   string `json:"step_kind,omitempty"`
	From       string `json:"from,omitempty"`
	Service    string `json:"service,omitempty"`
	Method     string `json:"method,omitempty"`
	URL        string `json:"url,omitempty"`
	Status     int    `json:"status,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Request    string `json:"request,omitempty"`
	Response   string `json:"response,omitempty"`
	Log        string `json:"log,omitempty"`
	Verdict    string `json:"verdict,omitempty"`
	Detail     string `json:"detail,omitempty"`

	BaseURL     string `json:"base_url,omitempty"`
	Environment string `json:"environment,omitempty"`
	Port        string `json:"port,omitempty"`
	LogSource   string `json:"log_source,omitempty"`

	Severity   string `json:"severity,omitempty"`
	Issue      string `json:"issue,omitempty"`
	Where      string `json:"where,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

// Límites por corrida (protegen la base y el visor ante un agente desbocado).
const (
	MaxCases    = 50
	MaxSteps    = 2000
	MaxFindings = 200
)

var validVerdicts = map[string]bool{"ok": true, "warn": true, "error": true, "info": true}

// NewRun crea una corrida en estado "grabando".
func NewRun(id, project, title, environment, agent string, now time.Time) *Run {
	ts := now.UTC().Format(time.RFC3339)
	return &Run{
		ID: id, Project: strings.TrimSpace(project), Title: strings.TrimSpace(title),
		Environment: strings.TrimSpace(environment), Agent: strings.TrimSpace(agent),
		Status: StatusRecording, Started: ts, Updated: ts,
		Services: []Service{}, Cases: []Case{}, Findings: []Finding{},
	}
}

// TotalSteps cuenta los pasos de todos los casos.
func (r *Run) TotalSteps() int {
	n := 0
	for _, c := range r.Cases {
		n += len(c.Steps)
	}
	return n
}

func (r *Run) caseIndex(n int) int {
	for i, c := range r.Cases {
		if c.N == n {
			return i
		}
	}
	return -1
}

// currentCase devuelve el índice del caso indicado o del último; crea el caso 1 si no hay.
func (r *Run) currentCase(n int) (int, error) {
	if n > 0 {
		if i := r.caseIndex(n); i >= 0 {
			return i, nil
		}
		return -1, fmt.Errorf("no existe el caso %d (abre uno con kind:\"case\")", n)
	}
	if len(r.Cases) == 0 {
		r.Cases = append(r.Cases, Case{N: 1, Title: "Caso 1", Steps: []Step{}})
	}
	return len(r.Cases) - 1, nil
}

// AddSteps agrega pasos (ya enmascarados) a un caso, numerándolos.
func (r *Run) AddSteps(caseN int, steps []Step, now time.Time) (int, error) {
	if r.TotalSteps()+len(steps) > MaxSteps {
		return 0, fmt.Errorf("la corrida llegaría a más de %d pasos", MaxSteps)
	}
	i, err := r.currentCase(caseN)
	if err != nil {
		return 0, err
	}
	for _, st := range steps {
		st.Seq = len(r.Cases[i].Steps) + 1
		if st.Time == "" {
			st.Time = now.UTC().Format(time.RFC3339)
		}
		if !validVerdicts[st.Verdict] {
			st.Verdict = verdictFromStatus(st.Status)
		}
		r.Cases[i].Steps = append(r.Cases[i].Steps, st)
	}
	return r.Cases[i].N, nil
}

func verdictFromStatus(status int) string {
	switch {
	case status >= 500:
		return "error"
	case status >= 400:
		return "warn"
	case status > 0:
		return "ok"
	default:
		return "info"
	}
}

// ErrFinished indica que la corrida ya se cerró.
var ErrFinished = errors.New("la corrida E2E ya terminó; inicia otra con e2e_start")

// Apply aplica un evento a la corrida. El enmascarado se hace antes (ver Manager).
// Devuelve el número de caso afectado (o 0).
func (r *Run) Apply(ev Event, now time.Time) (int, error) {
	if r.Status == StatusFinished {
		return 0, ErrFinished
	}
	r.Updated = now.UTC().Format(time.RFC3339)
	switch strings.ToLower(strings.TrimSpace(ev.Kind)) {
	case "case":
		if i := r.caseIndex(ev.Case); ev.Case > 0 && i >= 0 {
			// Actualiza un caso existente (p. ej. su resultado al cerrarlo).
			c := &r.Cases[i]
			if ev.Title != "" {
				c.Title = ev.Title
			}
			if ev.Expected != "" {
				c.Expected = ev.Expected
			}
			if ev.Result != "" {
				c.Result = ev.Result
			}
			return c.N, nil
		}
		if len(r.Cases) >= MaxCases {
			return 0, fmt.Errorf("máximo %d casos por corrida", MaxCases)
		}
		n := len(r.Cases) + 1
		title := strings.TrimSpace(ev.Title)
		if title == "" {
			title = fmt.Sprintf("Caso %d", n)
		}
		r.Cases = append(r.Cases, Case{N: n, Title: title, Expected: ev.Expected, Result: ev.Result, Steps: []Step{}})
		return n, nil

	case "step":
		kind := strings.ToLower(strings.TrimSpace(ev.StepKind))
		if kind == "" {
			kind = "request"
			if ev.URL == "" && ev.Method == "" {
				kind = "note"
			}
		}
		st := Step{
			Kind: kind, Title: ev.Title, From: ev.From, Service: ev.Service,
			Method: strings.ToUpper(ev.Method), URL: ev.URL, Status: ev.Status, DurationMs: ev.DurationMs,
			Request: ev.Request, Response: ev.Response, Log: ev.Log, Verdict: ev.Verdict, Detail: ev.Detail,
		}
		return r.AddSteps(ev.Case, []Step{st}, now)

	case "service":
		name := strings.TrimSpace(ev.Service)
		if name == "" {
			name = strings.TrimSpace(ev.Title)
		}
		if name == "" {
			return 0, errors.New("service requiere el nombre en service")
		}
		svc := Service{Name: name, BaseURL: ev.BaseURL, Environment: ev.Environment, Port: ev.Port, LogSource: ev.LogSource}
		for i := range r.Services {
			if strings.EqualFold(r.Services[i].Name, name) {
				mergeService(&r.Services[i], svc)
				return 0, nil
			}
		}
		r.Services = append(r.Services, svc)
		return 0, nil

	case "finding":
		if strings.TrimSpace(ev.Issue) == "" {
			return 0, errors.New("finding requiere issue (qué falta o qué está mal)")
		}
		if len(r.Findings) >= MaxFindings {
			return 0, fmt.Errorf("máximo %d hallazgos por corrida", MaxFindings)
		}
		sev := strings.ToLower(ev.Severity)
		if sev != "info" && sev != "error" {
			sev = "warning"
		}
		r.Findings = append(r.Findings, Finding{
			N: len(r.Findings) + 1, Service: ev.Service, Severity: sev,
			Issue: ev.Issue, Where: ev.Where, Suggestion: ev.Suggestion,
		})
		return 0, nil

	case "note":
		text := strings.TrimSpace(ev.Detail)
		if text == "" {
			text = strings.TrimSpace(ev.Title)
		}
		if text == "" {
			return 0, errors.New("note requiere detail")
		}
		r.Notes = append(r.Notes, text)
		return 0, nil
	}
	return 0, fmt.Errorf("kind desconocido %q (usa case, step, service, finding o note)", ev.Kind)
}

func mergeService(dst *Service, src Service) {
	if src.BaseURL != "" {
		dst.BaseURL = src.BaseURL
	}
	if src.Environment != "" {
		dst.Environment = src.Environment
	}
	if src.Port != "" {
		dst.Port = src.Port
	}
	if src.LogSource != "" {
		dst.LogSource = src.LogSource
	}
}
