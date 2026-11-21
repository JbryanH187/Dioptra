package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dioptra/internal/contract"
	"dioptra/internal/cost"
	"dioptra/internal/layoutagent"
	"dioptra/internal/mcpserver"
	"dioptra/internal/render"
	"dioptra/internal/store"
)

// requiredMCPTools es el contrato MCP mínimo que todo binario Dioptra debe exponer.
var requiredMCPTools = []string{"get_layout_contract", "get_report_state", "list_reports", "publish_report"}

// splitPositional permite banderas antes o después del argumento posicional.
func splitPositional(args []string, boolFlags map[string]bool) (flagArgs []string, positional string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
			name := strings.TrimLeft(a, "-")
			if !strings.Contains(a, "=") && !boolFlags[name] && i+1 < len(args) {
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
		} else if positional == "" {
			positional = a
		}
	}
	return flagArgs, positional
}

// runRender maqueta un .md desde una terminal desnuda. Prioridad del sintetizador local:
// DIOPTRA_LAYOUT_CLI (CLI ya autenticada, sin API keys) → adaptador HTTP opcional.
// Sin ninguno publica por goldmark. El resultado viaja como blocks_override y el
// servidor lo vuelve a auditar con el Guardián de Fidelidad.
func runRender(args []string) error {
	flagArgs, fileArg := splitPositional(args, map[string]bool{"no-browser": true})
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	port := fs.Int("port", defaultPort, "Puerto local del demonio dioptra serve")
	dir := fs.String("dir", store.DefaultReportsDir(), "Directorio raíz de reportes Markdown")
	project := fs.String("project", "", "Proyecto asociado al reporte")
	outPath := fs.String("out", "", "Guardar además el sobre sintetizado en este archivo JSON")
	noBrowser := fs.Bool("no-browser", false, "No lanzar el navegador automáticamente")
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if fileArg == "" {
		return errors.New("uso: dioptra render <archivo.md> [--project nombre]")
	}

	rawBytes, err := os.ReadFile(fileArg)
	if err != nil {
		return fmt.Errorf("leyendo archivo %s: %w", fileArg, err)
	}
	mdContent := string(rawBytes)
	fm := render.ParseFrontmatter(mdContent)
	title := titleFromMarkdown(mdContent, fm.Title, fileArg)
	proj := firstNonEmpty(strings.TrimSpace(*project), fm.Project, filepath.Base(filepath.Dir(mustAbs(fileArg))))
	pubIn := mcpserver.PublishReportInput{
		IdempotencyKey: "file:" + filepath.ToSlash(mustAbs(fileArg)),
		Title:          title,
		Project:        proj,
		Markdown:       mdContent,
	}

	cfg := layoutagent.ConfigFromEnv()
	resolved, resolveErr := cfg.Resolve()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.EffectiveTimeout()+20*time.Second)
	defer cancel()

	var synthErr error
	var localMeta *contract.LayoutMeta
	fmt.Printf("Agente Único de Dioptra · %s\n", layoutagent.PromptVersion)
	if resolveErr != nil {
		fmt.Printf("  ○ %v\n", resolveErr)
		fmt.Println("  → Publicando por ruta rápida goldmark. Para maquetación interactiva sin API keys:")
		fmt.Println("    · terminal: $env:DIOPTRA_LAYOUT_CLI=\"claude -p\"  (cualquier CLI autenticada que lea stdin)")
		fmt.Println("    · agente MCP: get_layout_contract + publish_report con blocks_override")
	} else {
		pubIn.Model = resolved.Model
		fmt.Printf("  Proveedor:   %s (%s)\n", resolved.Provider, resolved.ProviderSource)
		fmt.Printf("  Modelo:      %s\n", resolved.Model)
		fmt.Printf("  Endpoint:    %s\n", resolved.Endpoint)
		fmt.Printf("  Entrada:     %d bytes · tiempo límite %s\n", len(mdContent), cfg.EffectiveTimeout())

		wall := time.Now()
		res, err := layoutagent.Synthesize(ctx, cfg, layoutagent.Input{
			Title: title, Project: proj, TS: time.Now().Format(time.RFC3339), Markdown: mdContent,
		})
		synthErr = err
		wallMs := time.Since(wall).Milliseconds()
		if res != nil {
			m := res.Meta
			localMeta = &m
			fmt.Printf("  Latencia:    %d ms en proveedor · %d ms total (incluye línea base y Guardián) · %d intento(s)\n", m.LatencyMs, wallMs, m.Attempts)
			if m.InputTokens+m.OutputTokens > 0 {
				fmt.Printf("  Tokens:      entrada %d · salida %d · caché leída %d · caché escrita %d\n", m.InputTokens, m.OutputTokens, m.CacheReadTokens, m.CacheWriteTokens)
			}
			if m.Coverage > 0 {
				fmt.Printf("  Fidelidad:   cobertura léxica %.1f%% · %d reparación(es) del Guardián\n", m.Coverage*100, m.FidelityRepairs)
			}
		}
		if synthErr != nil {
			fmt.Printf("  ✖ Fallo del sintetizador: %v\n", synthErr)
			fmt.Println("  → Degradando a la ruta rápida goldmark (el reporte se publica igualmente).")
		} else {
			blocks := make([]any, 0, len(res.Envelope.Report.Blocks))
			for _, b := range res.Envelope.Report.Blocks {
				blocks = append(blocks, b)
			}
			pubIn.BlocksOverride = blocks
			printWarnings("Advertencias ADR 1 / Guardián (local)", res.Warnings)
			if *outPath != "" {
				if raw, err := json.MarshalIndent(res.Envelope, "", "  "); err == nil {
					if err := os.WriteFile(*outPath, raw, 0o644); err != nil {
						fmt.Printf("  ⚠ No se pudo escribir %s: %v\n", *outPath, err)
					} else {
						fmt.Printf("  Sobre JSON:  %s\n", *outPath)
					}
				}
			}
		}
	}

	if err := ensureLocalDaemon(ctx, *port, expandHomePath(*dir)); err != nil {
		return err
	}
	bridge := &mcpserver.HTTPBridgeBackend{BaseURL: fmt.Sprintf("http://127.0.0.1:%d", *port)}
	pubOut, err := bridge.PublishReport(ctx, pubIn)
	if err != nil {
		return err
	}
	fmt.Printf("Reporte publicado: %s (modo servidor: %s", pubOut.ID, pubOut.ModeUsed)
	if pubOut.Action != "" {
		fmt.Printf(" · %s · rev %d", pubOut.Action, max(pubOut.Revision, 1))
	}
	fmt.Println(")")
	if pubOut.ChangeNote != "" {
		fmt.Printf("Cambio aplicado:   %s\n", pubOut.ChangeNote)
	}
	fmt.Printf("Fuente Markdown:   %s\n", pubOut.MarkdownPath)
	fmt.Printf("URL del visor:     %s\n", pubOut.URL)
	if localMeta != nil && synthErr == nil {
		localCost := cost.Compute(cost.ComputeParams{
			Mode: "layout_agent", Model: resolved.Model, Markdown: mdContent, LayoutMeta: localMeta,
		})
		fmt.Printf("Costo / delta:     %s\n", localCost.ConsoleLine)
	} else if pubOut.Cost != nil && pubOut.Cost.ConsoleLine != "" {
		fmt.Printf("Costo / delta:     %s\n", pubOut.Cost.ConsoleLine)
	}
	if f := pubOut.Fidelity; f != nil {
		fmt.Printf("Guardián servidor: %s · cobertura %.1f%% · %d reparación(es)\n", f.Verdict, f.Coverage*100, f.Repairs)
	}
	if len(pubIn.BlocksOverride) > 0 {
		printWarnings("Avisos de revalidación en servidor", pubOut.Warnings)
	}
	if !*noBrowser {
		_ = openBrowserURL(pubOut.URL)
	}
	if synthErr != nil {
		return fmt.Errorf("render completado en modo degradado: %w", synthErr)
	}
	return nil
}

func printWarnings(label string, ws []contract.Warning) {
	if len(ws) == 0 {
		return
	}
	fmt.Printf("  %s: %d\n", label, len(ws))
	for i, w := range ws {
		if i >= 25 {
			fmt.Printf("    … %d más\n", len(ws)-i)
			break
		}
		fmt.Printf("    [%s] %s — %s\n", w.Action, w.Path, w.Reason)
	}
}

// runDoctor valida lo indispensable para el flujo MCP sin credenciales (SQLite, contrato
// MCP y demonio local). Los sintetizadores externos se reportan como estrictamente opcionales.
func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	port := fs.Int("port", defaultPort, "Puerto local del demonio dioptra serve")
	dir := fs.String("dir", store.DefaultReportsDir(), "Directorio raíz de reportes Markdown")
	probe := fs.Bool("probe", false, "Ejecutar una síntesis real con el sintetizador opcional configurado (consume tokens)")
	_ = fs.Bool("offline", false, "Obsoleto: doctor ya no contacta proveedores salvo con --probe")
	if err := fs.Parse(args); err != nil {
		return err
	}
	reportsDir := expandHomePath(*dir)
	failures := 0
	ok := func(f string, a ...any) { fmt.Printf("  ✔ "+f+"\n", a...) }
	info := func(f string, a ...any) { fmt.Printf("  ○ "+f+"\n", a...) }
	fail := func(f string, a ...any) { failures++; fmt.Printf("  ✖ "+f+"\n", a...) }

	fmt.Println("DioptraEngine doctor")

	// 1. Índice SQLite
	fmt.Println("[1/4] Índice SQLite")
	dctx, dcancel := context.WithTimeout(context.Background(), 10*time.Second)
	if st, err := store.Open(reportsDir); err != nil {
		fail("no se pudo abrir el almacén %s: %v", reportsDir, err)
	} else {
		if d, err := st.Diagnose(dctx); err != nil {
			fail("diagnóstico SQLite: %v", err)
		} else if !strings.EqualFold(d.QuickCheck, "ok") {
			fail("quick_check=%s (%s)", d.QuickCheck, d.DBPath)
		} else {
			ok("%s", d.DBPath)
			ok("quick_check=ok · journal=%s · FTS5=%v · %d reporte(s) indexado(s)", d.JournalMode, d.FTS, d.Reports)
		}
		_ = st.Close()
	}
	dcancel()

	// 2. Contrato MCP compilado en este binario (sesión en memoria, sin red)
	fmt.Println("[2/4] Contrato MCP")
	if tools, contractVersion, err := probeInMemoryMCP(); err != nil {
		fail("contrato MCP: %v", err)
	} else {
		ok("herramientas: %s", strings.Join(tools, ", "))
		ok("recurso %s · get_layout_contract %s · %d tokens CSS", layoutagent.LayoutContractURI, contractVersion, len(layoutagent.CanvasTokens()))
	}

	// 3. Demonio local
	fmt.Println("[3/4] Demonio local")
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", *port)
	if health, err := fetchHealth(baseURL + "/api/health"); err == nil {
		tools, terr := listRemoteMCPTools(baseURL + "/mcp")
		switch {
		case terr != nil:
			fail("dioptra serve activo pero /mcp no responde: %v", terr)
		case !containsAll(tools, requiredMCPTools):
			fail("dioptra serve activo con un binario anterior (faltan herramientas de %s): reinícialo", strings.Join(requiredMCPTools, ", "))
		default:
			ok("dioptra serve activo en 127.0.0.1:%d · /mcp con %d herramientas (dir: %v)", *port, len(tools), health["dir"])
		}
	} else if ln, lerr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port)); lerr == nil {
		_ = ln.Close()
		ok("puerto 127.0.0.1:%d libre · el demonio se inicia bajo demanda (dioptra mcp/open/render)", *port)
	} else {
		fail("puerto 127.0.0.1:%d ocupado por otro proceso que no es Dioptra: %v", *port, lerr)
	}
	if line, found, aerr := readAutostartEntry(); aerr == nil {
		if found {
			ok("arranque automático al iniciar sesión: activado")
			if exe := autostartExecutable(line); exe != "" {
				if _, serr := os.Stat(exe); serr != nil {
					fail("el arranque automático apunta a un binario que ya no existe (%s): ejecuta «dioptra autostart on»", exe)
				}
			}
		} else {
			info("arranque automático al iniciar sesión: desactivado (dioptra autostart on)")
		}
	}

	// 4. Sintetizador local para `dioptra render` (estrictamente opcional)
	fmt.Println("[4/4] Sintetizador local para `dioptra render` (opcional)")
	info("flujo MCP sin credenciales: el agente anfitrión usa get_layout_contract + blocks_override")
	cfg := layoutagent.ConfigFromEnv()
	if resolved, err := cfg.Resolve(); err != nil {
		info("%v", err)
	} else {
		info("proveedor %s (%s) · modelo %s · %s · tiempo límite %s", resolved.Provider, resolved.ProviderSource, resolved.Model, resolved.Endpoint, cfg.EffectiveTimeout())
		if *probe {
			pctx, pcancel := context.WithTimeout(context.Background(), cfg.EffectiveTimeout()+5*time.Second)
			res, perr := layoutagent.Synthesize(pctx, cfg, layoutagent.Input{
				Title:    "Diagnóstico Dioptra",
				Project:  "dioptra-doctor",
				TS:       time.Now().Format(time.RFC3339),
				Markdown: "# Diagnóstico Dioptra\n\nLatencia p95 medida: 120 ms sobre 3 servicios.\n\n- [ ] Confirmar conectividad del sintetizador\n",
			})
			pcancel()
			if perr != nil {
				info("⚠ síntesis de prueba falló (no afecta al flujo MCP): %v", perr)
			} else {
				m := res.Meta
				info("síntesis de prueba OK en %d ms · %d bloque(s) · %d aviso(s) · tokens %d/%d", m.LatencyMs, len(res.Envelope.Report.Blocks), len(res.Warnings), m.InputTokens, m.OutputTokens)
			}
		} else {
			info("usa --probe para ejecutar una síntesis real de prueba")
		}
	}

	if failures > 0 {
		return fmt.Errorf("%d comprobación(es) fallida(s)", failures)
	}
	fmt.Println("Estado: OK (100% saludable para el flujo MCP)")
	return nil
}

// doctorBackend satisface mcpserver.Backend sin efectos: el diagnóstico solo lista
// herramientas y lee el contrato, nunca publica.
type doctorBackend struct{}

var errDoctorBackend = errors.New("backend de diagnóstico: operación no disponible")

func (doctorBackend) PublishReport(context.Context, mcpserver.PublishReportInput) (*mcpserver.PublishReportOutput, error) {
	return nil, errDoctorBackend
}
func (doctorBackend) ListReports(context.Context, mcpserver.ListReportsInput) (*mcpserver.ListReportsOutput, error) {
	return nil, errDoctorBackend
}
func (doctorBackend) GetReportState(context.Context, mcpserver.GetReportStateInput) (*mcpserver.GetReportStateOutput, error) {
	return nil, errDoctorBackend
}

func probeInMemoryMCP() ([]string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	srv := mcpserver.NewMCPServer(doctorBackend{})
	clientT, serverT := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		return nil, "", err
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "dioptra-doctor", Version: "1"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		return nil, "", err
	}
	defer cs.Close()

	tl, err := cs.ListTools(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	var names []string
	for _, t := range tl.Tools {
		names = append(names, t.Name)
	}
	sort.Strings(names)
	if !containsAll(names, requiredMCPTools) {
		return names, "", fmt.Errorf("faltan herramientas: se esperaban %s", strings.Join(requiredMCPTools, ", "))
	}

	rr, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: layoutagent.LayoutContractURI})
	if err != nil {
		return names, "", fmt.Errorf("leyendo %s: %w", layoutagent.LayoutContractURI, err)
	}
	if len(rr.Contents) == 0 || !strings.Contains(rr.Contents[0].Text, contract.CompactSchemaB) {
		return names, "", fmt.Errorf("recurso %s sin esquema del Contrato B", layoutagent.LayoutContractURI)
	}

	cr, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "get_layout_contract", Arguments: map[string]any{}})
	if err != nil || cr.IsError {
		return names, "", fmt.Errorf("get_layout_contract falló: %v", err)
	}
	raw, _ := json.Marshal(cr.StructuredContent)
	var lc layoutagent.LayoutContract
	if err := json.Unmarshal(raw, &lc); err != nil || lc.Rules == "" || lc.Schema == "" {
		return names, "", errors.New("get_layout_contract devolvió un contrato incompleto")
	}
	return names, lc.Version, nil
}

func listRemoteMCPTools(endpoint string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "dioptra-doctor", Version: "1"}, nil).
		Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint}, nil)
	if err != nil {
		return nil, err
	}
	defer cs.Close()
	tl, err := cs.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, t := range tl.Tools {
		names = append(names, t.Name)
	}
	return names, nil
}

func containsAll(have, want []string) bool {
	set := make(map[string]bool, len(have))
	for _, h := range have {
		set[h] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

func fetchHealth(healthURL string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var out map[string]any
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// runStats imprime el acumulado histórico de tokens y diferencia en USD guardado en SQLite.
func runStats(args []string) error {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	dir := fs.String("dir", store.DefaultReportsDir(), "Directorio raíz de reportes Markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	st, err := store.Open(expandHomePath(*dir))
	if err != nil {
		return err
	}
	defer st.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sum, err := st.CostSummary(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("DioptraEngine stats · %s\n", st.BaseDir())
	if sum.TotalReports == 0 {
		fmt.Println("  Aún no hay publicaciones medidas en publish_metrics. Publica o abre un reporte para iniciar el histórico.")
		return nil
	}
	fmt.Printf("  Total medido:   %d reporte(s)\n", sum.TotalReports)
	fmt.Printf("  Consola (base): %d tok salida · $%.4f USD\n", sum.BaselineOutTokens, sum.BaselineUSD)
	fmt.Printf("  Con Dioptra:    %d tok entrada + %d tok salida · $%.4f USD\n", sum.DioptraInTokens, sum.DioptraOutTokens, sum.DioptraUSD)
	fmt.Printf("  Delta neto:     +%d tok salida · +$%.4f USD extra acumulados\n", sum.ExtraOutTokens, sum.DeltaUSD)
	fmt.Println("")
	fmt.Println("  Por modo:")
	for _, m := range sum.ByMode {
		fmt.Printf("    - %-15s %3d rep | +%5.1f%% tok salida promedio | Consola $%.4f -> Dioptra $%.4f (+$%.4f USD)\n",
			m.Mode, m.Reports, m.AvgDeltaPct, m.BaselineUSD, m.DioptraUSD, m.DeltaUSD)
	}
	return nil
}

// titleFromMarkdown elige el título de un .md publicado desde la CLI:
// frontmatter `title`, luego el primer encabezado `# H1` (fuera de bloques de
// código) y, en último caso, el nombre del archivo sin extensión.
func titleFromMarkdown(md, fmTitle, fileArg string) string {
	if t := strings.TrimSpace(fmTitle); t != "" {
		return t
	}
	inFence := false
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence && strings.HasPrefix(trimmed, "# ") {
			if t := strings.TrimSpace(strings.TrimRight(trimmed[2:], "#")); t != "" {
				return t
			}
		}
	}
	base := filepath.Base(fileArg)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
