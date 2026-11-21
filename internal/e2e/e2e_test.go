package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRedactor_DatosSensibles(t *testing.T) {
	r := &Redactor{}
	in := `pago con 4111 1111 1111 1111, correo ana.lopez@example.com, tel 667 123 4567, ` +
		`CURP LOPA900101MSLRRN09, Bearer abcdefghijklmnop, password=Secreta123 y folio 123456789012`
	out := r.Text(in)
	for _, leak := range []string{"4111 1111 1111 1111", "ana.lopez", "123 4567", "LOPA900101MSLRRN09", "abcdefghijklmnop", "Secreta123"} {
		if strings.Contains(out, leak) {
			t.Errorf("no ocultó %q en: %s", leak, out)
		}
	}
	for _, keep := range []string{"****1111", "a***@example.com", "***4567", "123456789012"} {
		if !strings.Contains(out, keep) {
			t.Errorf("esperaba conservar %q en: %s", keep, out)
		}
	}
	if r.Count < 6 {
		t.Errorf("Count = %d, esperaba >= 6", r.Count)
	}
}

func TestRedactor_BodyJSONYFormulario(t *testing.T) {
	r := &Redactor{}
	out := r.Body(`{"usuario":"ana","password":"x1","datos":{"token":"t","monto":300},"tarjetas":[{"numero":"4111111111111111"}]}`)
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("el body enmascarado debe seguir siendo JSON: %v\n%s", err, out)
	}
	if v["password"] != "[oculto]" || v["datos"].(map[string]any)["token"] != "[oculto]" {
		t.Errorf("no ocultó llaves sensibles: %s", out)
	}
	if v["datos"].(map[string]any)["monto"].(float64) != 300 {
		t.Errorf("no debe tocar datos de negocio: %s", out)
	}
	if strings.Contains(out, "4111111111111111") {
		t.Errorf("no ocultó la tarjeta: %s", out)
	}
	form := r.Body("user=ana&password=x1&monto=300")
	if form != "user=ana&password=[oculto]&monto=300" {
		t.Errorf("formulario: %s", form)
	}
	if got := r.Body(strings.Repeat("a", MaxBodyBytes+10)); !strings.HasSuffix(got, "[recortado]") {
		t.Error("debe recortar cuerpos enormes")
	}
}

func TestParseHAR_PasosServiciosYEnmascarado(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "abono.har"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	red := &Redactor{}
	steps, services, stats, err := ParseHAR(f, red)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Entries != 7 || stats.Steps != 4 || stats.Skipped != 3 {
		t.Fatalf("stats = %+v (esperaba 7 entradas, 4 pasos: documento + 3 APIs; 3 omitidos: js, png, OPTIONS)", stats)
	}
	if len(services) != 4 {
		t.Errorf("servicios = %d, esperaba 4 hosts", len(services))
	}
	clientes := steps[1]
	if clientes.Service != "localhost:8081" || clientes.From != "abonos.qa.local" || clientes.Status != 200 || clientes.DurationMs != 180 {
		t.Errorf("paso clientes = %+v", clientes)
	}
	if !strings.Contains(clientes.Request, "X-Correlation-Id: corr-77") || strings.Contains(clientes.Request, "User-Agent") {
		t.Errorf("cabeceras conservadas: %s", clientes.Request)
	}
	all, _ := json.Marshal(steps)
	if !strings.Contains(clientes.Response, `"nombre": "C***"`) {
		t.Errorf("el nombre del cliente debe quedar con su inicial: %s", clientes.Response)
	}
	for _, leak := range []string{"eyJhbGci", "4111111111111111", "cliente.prueba@", "6671234567", `"1234"`, "abc123def456", "Cliente Prueba"} {
		if strings.Contains(string(all), leak) {
			t.Errorf("el HAR filtró %q", leak)
		}
	}
	abono := steps[3]
	if abono.Status != 422 || abono.Verdict != "warn" || !strings.Contains(abono.Response, "NSF") {
		t.Errorf("paso abono = %+v", abono)
	}
}

func TestRun_ApplyYLimites(t *testing.T) {
	now := time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)
	r := NewRun("e2e_x", "abonos", "Abono", "QA", "Genius", now)
	if _, err := r.Apply(Event{Kind: "step", Method: "get", URL: "/x", Status: 500}, now); err != nil {
		t.Fatal(err)
	}
	if len(r.Cases) != 1 || r.Cases[0].Steps[0].Verdict != "error" || r.Cases[0].Steps[0].Method != "GET" {
		t.Fatalf("un paso sin caso abre el caso 1: %+v", r.Cases)
	}
	if n, _ := r.Apply(Event{Kind: "case", Title: "Saldo insuficiente"}, now); n != 2 {
		t.Errorf("segundo caso = %d", n)
	}
	if n, _ := r.Apply(Event{Kind: "case", Case: 1, Result: "Abono aplicado"}, now); n != 1 || r.Cases[0].Result != "Abono aplicado" {
		t.Error("kind case con número existente actualiza el resultado")
	}
	if _, err := r.Apply(Event{Kind: "step", Case: 9}, now); err == nil {
		t.Error("caso inexistente debe fallar")
	}
	r.Apply(Event{Kind: "service", Service: "api-pagos", BaseURL: "http://localhost:8095"}, now)
	r.Apply(Event{Kind: "service", Service: "API-PAGOS", Port: "8095", LogSource: "logs/pagos.log"}, now)
	if len(r.Services) != 1 || r.Services[0].LogSource != "logs/pagos.log" {
		t.Errorf("servicio repetido se fusiona: %+v", r.Services)
	}
	if _, err := r.Apply(Event{Kind: "finding"}, now); err == nil {
		t.Error("finding sin issue debe fallar")
	}
	if _, err := r.Apply(Event{Kind: "rara"}, now); err == nil {
		t.Error("kind desconocido debe fallar")
	}
	r.Status = StatusFinished
	if _, err := r.Apply(Event{Kind: "note", Detail: "x"}, now); !errors.Is(err, ErrFinished) {
		t.Error("una corrida terminada no acepta eventos")
	}
}

func TestMarkdown_EstructuraYChecklistConservada(t *testing.T) {
	now := time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)
	r := NewRun("e2e_x", "abonos", "Abono", "QA", "Genius", now)
	r.Apply(Event{Kind: "service", Service: "api-pagos", BaseURL: "http://localhost:8095", Environment: "QA"}, now)
	r.Apply(Event{Kind: "case", Title: "Abono con NSF", Expected: "rechazo con NSF"}, now)
	r.Apply(Event{Kind: "step", From: "abonos-wp", Service: "api-pagos", Method: "POST", URL: "http://localhost:8095/abonos", Status: 422,
		Request: `{"monto":300}`, Response: "```raro```", Log: "WARN NSF"}, now)
	r.Apply(Event{Kind: "finding", Service: "api-pagos", Issue: "no registra el folio", Suggestion: "loguear folio y regla NSF"}, now)
	md := Markdown(r, map[string]bool{NormalizeCheck("api-pagos: no registra el folio → loguear folio y regla NSF [warning]"): true})
	for _, want := range []string{
		"# E2E · Abono", "```kpi", "Fallos | 0", "Advertencias | 1 | | | warning", "```mermaid\nflowchart LR",
		"n_abonos_wp -->|1| n_api_pagos", "| api-pagos | `http://localhost:8095` | QA |", "## Caso 1 · Abono con NSF", "| 1 | ", "POST /abonos",
		"<details><summary>Paso 1 · POST /abonos</summary>", "````\n```raro```\n````", "## Calidad de logs",
		"- [x] **api-pagos**: no registra el folio → loguear folio y regla NSF [warning]",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("falta %q en:\n%s", want, md)
		}
	}
}

type memStore struct{ docs map[string][]byte }

func (m *memStore) SaveE2ERun(_ context.Context, id, _, _, _ string, doc []byte) error {
	m.docs[id] = append([]byte(nil), doc...)
	return nil
}

func (m *memStore) LoadE2ERun(_ context.Context, id string) ([]byte, error) {
	d, ok := m.docs[id]
	if !ok {
		return nil, errors.New("no existe")
	}
	return d, nil
}

type memPub struct {
	published []string
	checked   map[string]bool
}

func (p *memPub) PublishE2E(_ context.Context, r *Run, md string) (string, string, error) {
	p.published = append(p.published, md)
	return "r_e2e", "http://127.0.0.1:7474/?report=r_e2e", nil
}

func (p *memPub) CheckedItems(context.Context, string) map[string]bool { return p.checked }

func TestManager_FlujoCompleto(t *testing.T) {
	ctx := context.Background()
	st, pub := &memStore{docs: map[string][]byte{}}, &memPub{}
	m := &Manager{Store: st, Pub: pub, Now: func() time.Time { return time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC) }}

	start, err := m.Start(ctx, "abonos", "Abono · QA", "QA", "Genius")
	if err != nil || !strings.HasPrefix(start.Run, "e2e_") || start.URL == "" || start.Status != StatusRecording {
		t.Fatalf("Start = %+v, %v", start, err)
	}
	if _, err := m.Record(ctx, start.Run, Event{Kind: "service", Service: "api-clientes", BaseURL: "http://localhost:8081"}); err != nil {
		t.Fatal(err)
	}
	m.Record(ctx, start.Run, Event{Kind: "case", Title: "Abono exitoso"})
	har, err := m.IngestHAR(ctx, start.Run, 1, filepath.Join("testdata", "abono.har"))
	if err != nil {
		t.Fatal(err)
	}
	if har.Added != 4 || har.HAR.Skipped != 3 || har.Redactions == 0 {
		t.Errorf("IngestHAR = %+v", har)
	}
	rec, err := m.Record(ctx, start.Run, Event{Kind: "step", Service: "localhost:8095", StepKind: "log", Log: "pago token=abcdef123 tarjeta 4111111111111111"})
	if err != nil || !strings.Contains(rec.Message, "datos sensibles ocultos") {
		t.Fatalf("Record = %+v, %v", rec, err)
	}
	doc := string(st.docs[start.Run])
	if strings.Contains(doc, "abcdef123") || strings.Contains(doc, "4111111111111111") {
		t.Error("nada sensible llega a la base")
	}
	if !strings.Contains(doc, `"service":"api-clientes"`) {
		t.Error("el host localhost:8081 del HAR debe tomar el nombre registrado api-clientes")
	}
	if _, err := m.IngestHAR(ctx, start.Run, 1, "notas.txt"); err == nil {
		t.Error("solo acepta archivos .har")
	}
	fin, err := m.Finish(ctx, start.Run, "Abono aplicado; NSF detectado en el caso 2.")
	if err != nil || fin.Status != StatusFinished || fin.Steps != 5 {
		t.Fatalf("Finish = %+v, %v", fin, err)
	}
	if _, err := m.Record(ctx, start.Run, Event{Kind: "note", Detail: "x"}); !errors.Is(err, ErrFinished) {
		t.Error("tras Finish no se registra nada")
	}
	if last := pub.published[len(pub.published)-1]; !strings.Contains(last, "Estado: **terminado**") || !strings.Contains(last, "## Resumen") {
		t.Errorf("el último reporte publicado:\n%s", last)
	}
}
