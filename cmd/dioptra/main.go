package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"dioptra/internal/layoutagent"
	"dioptra/internal/mcpserver"
	"dioptra/internal/render"
	"dioptra/internal/server"
	"dioptra/internal/store"
)

const defaultPort = 7474

// Version se fija al compilar: -ldflags "-X main.Version=v1.0.0". "dev" = build local.
var Version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(2)
	}

	mcpserver.ServerVersion = Version
	subcmd := strings.ToLower(strings.TrimSpace(os.Args[1]))
	var err error

	switch subcmd {
	case "serve":
		err = runServe(os.Args[2:])
	case "mcp":
		err = runMCP(os.Args[2:])
	case "start":
		err = runStart(os.Args[2:])
	case "autostart":
		err = runAutostart(os.Args[2:])
	case "open":
		err = runOpen(os.Args[2:])
	case "render":
		err = runRender(os.Args[2:])
	case "doctor":
		err = runDoctor(os.Args[2:])
	case "stats":
		err = runStats(os.Args[2:])
	case "db":
		err = runDB(os.Args[2:])
	case "roots":
		err = runRoots(os.Args[2:])
	case "-v", "--version", "version":
		fmt.Fprintf(os.Stdout, "dioptra %s\n", Version)
		return
	case "-h", "--help", "help":
		printUsage(os.Stdout)
		return
	default:
		fmt.Fprintf(os.Stderr, "subcomando desconocido: %q\n\n", os.Args[1])
		printUsage(os.Stderr)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "dioptra %s: %v\n", subcmd, err)
		os.Exit(1)
	}
}

func printUsage(out *os.File) {
	fmt.Fprintln(out, "DioptraEngine — Visor técnico local y servidor MCP en binario único")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Uso:")
	fmt.Fprintln(out, "  dioptra serve [--port 7474] [--dir ~/.dioptra/reports] [--allow-host host1,host2]")
	fmt.Fprintln(out, "  dioptra mcp   [--port 7474] [--dir ~/.dioptra/reports]")
	fmt.Fprintln(out, "  dioptra start [--port 7474] [--dir ~/.dioptra/reports]   (demonio en segundo plano)")
	fmt.Fprintln(out, "  dioptra autostart on|off|status [--port 7474] [--dir ~/.dioptra/reports]   (al iniciar sesión, Windows)")
	fmt.Fprintln(out, "  dioptra open  [archivo.md] [--port 7474] [--dir ~/.dioptra/reports] [--project nombre] [--no-browser]")
	fmt.Fprintln(out, "  dioptra render <archivo.md> [--project nombre] [--port 7474] [--dir ~/.dioptra/reports] [--out sobre.json] [--no-browser]")
	fmt.Fprintln(out, "  dioptra doctor [--port 7474] [--dir ~/.dioptra/reports] [--probe]")
	fmt.Fprintln(out, "  dioptra stats  [--dir ~/.dioptra/reports]")
	fmt.Fprintln(out, "  dioptra db check|backup|reindex|alias list|set <alias> <canónica>|remove <alias> [--dir ~/.dioptra/reports]")
	fmt.Fprintln(out, "  dioptra roots list|set <proyecto> <ruta>|remove <proyecto> [--dir ~/.dioptra/reports]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Flujo MCP sin credenciales: el agente anfitrión llama get_layout_contract y envía blocks_override.")
	fmt.Fprintln(out, "Sintetizador local opcional para `render`: DIOPTRA_LAYOUT_CLI=\"claude -p\" (CLI autenticada que lee stdin)")
	fmt.Fprintln(out, "  o adaptador HTTP secundario: DIOPTRA_LAYOUT_API_KEY, DIOPTRA_LAYOUT_MODEL, DIOPTRA_LAYOUT_PROVIDER,")
	fmt.Fprintln(out, "  DIOPTRA_LAYOUT_API_URL, DIOPTRA_LAYOUT_TIMEOUT, DIOPTRA_LAYOUT_MAX_TOKENS, DIOPTRA_LAYOUT_TEMPERATURE")
}

func expandHomePath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return store.DefaultReportsDir()
	}
	if trimmed == "~" || strings.HasPrefix(trimmed, "~/") || strings.HasPrefix(trimmed, `~\`) {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, trimmed[1:])
		}
	}
	return trimmed
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fs.Int("port", defaultPort, "Puerto TCP loopback (127.0.0.1)")
	dir := fs.String("dir", store.DefaultReportsDir(), "Directorio raíz de reportes Markdown")
	allowHost := fs.String("allow-host", os.Getenv("DIOPTRA_ALLOW_HOSTS"), "Hosts adicionales explícitos separados por coma (ej. Tailscale MagicDNS)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	reportsDir := expandHomePath(*dir)
	st, err := store.Open(reportsDir)
	if err != nil {
		return err
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	_ = st.SyncMarkdownFromDisk(ctx)

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	baseURL := fmt.Sprintf("http://%s", addr)

	var extraHosts []string
	if strings.TrimSpace(*allowHost) != "" {
		extraHosts = strings.Split(*allowHost, ",")
	}

	appSrv := server.New(server.Options{
		Store:        st,
		BaseURL:      baseURL,
		LayoutCfg:    layoutagent.ConfigFromEnv(),
		AllowedHosts: extraHosts,
	})

	// Vigilante ligero de archivos .md nuevos depositados directamente en disco
	go watchDiskReports(ctx, st, appSrv.Hub())

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// Varias sesiones `dioptra mcp` intentan levantar `serve`: si el puerto ya lo atiende otra
		// instancia de Dioptra es el caso normal, no un error. Un solo aviso informativo y salida limpia.
		if isAddrInUse(err) && dioptraAlreadyServing(baseURL) {
			fmt.Fprintf(os.Stderr, "DioptraEngine ya está activo en %s; se reutiliza la instancia existente.\n", baseURL)
			return nil
		}
		return fmt.Errorf("escuchando en %s: %w", addr, err)
	}

	httpSrv := &http.Server{
		Handler:           appSrv,
		ReadHeaderTimeout: 10 * time.Second,
	}

	fmt.Fprintf(os.Stderr, "DioptraEngine activo en %s\n", baseURL)
	fmt.Fprintf(os.Stderr, "Endpoint MCP HTTP:  %s/mcp\n", baseURL)
	fmt.Fprintf(os.Stderr, "Almacén Markdown:   %s\n", st.BaseDir())

	errCh := make(chan error, 1)
	go func() {
		if serveErr := httpSrv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		} else {
			errCh <- nil
		}
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
		return nil
	case serveErr := <-errCh:
		return serveErr
	}
}

func watchDiskReports(ctx context.Context, st *store.Store, hub *server.SSEHub) {
	// Primera pasada: registra los mtimes actuales sin emitir eventos.
	if _, err := st.SyncDiskChanges(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "dioptra: sincronización inicial de disco: %v\n", err)
	}

	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			changes, err := st.SyncDiskChanges(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "dioptra: sincronización de disco: %v\n", err)
			}
			for _, ch := range changes {
				if ch.Action == "updated" {
					hub.Broadcast("report_updated", map[string]any{"summary": ch.Summary, "source": "disk"})
				} else {
					hub.Broadcast("report_published", map[string]any{"summary": ch.Summary})
				}
			}
		}
	}
}

func runMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(os.Stderr) // Jamás escribir mensajes auxiliares en stdout (reservado para JSON-RPC)
	port := fs.Int("port", defaultPort, "Puerto local del demonio dioptra serve")
	dir := fs.String("dir", store.DefaultReportsDir(), "Directorio raíz de reportes Markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}

	reportsDir := expandHomePath(*dir)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", *port)

	bridge := &mcpserver.HTTPBridgeBackend{
		BaseURL: baseURL,
		EnsureDaemon: func(ctx context.Context) error {
			return ensureLocalDaemon(ctx, *port, reportsDir)
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Intentar asegurar el demonio de forma proactiva al arrancar el puente stdio
	_ = ensureLocalDaemon(ctx, *port, reportsDir)

	return mcpserver.RunStdio(ctx, bridge)
}

func runOpen(args []string) error {
	// Permitimos banderas antes o después del argumento posicional [archivo.md]
	var flagArgs []string
	var fileArg string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
			if !strings.Contains(a, "=") && a != "--no-browser" && a != "-no-browser" && i+1 < len(args) {
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
		} else if fileArg == "" {
			fileArg = a
		}
	}

	fs := flag.NewFlagSet("open", flag.ContinueOnError)
	port := fs.Int("port", defaultPort, "Puerto local del demonio dioptra serve")
	dir := fs.String("dir", store.DefaultReportsDir(), "Directorio raíz de reportes Markdown")
	project := fs.String("project", "", "Proyecto asociado al reporte importado")
	noBrowser := fs.Bool("no-browser", false, "No lanzar el navegador automáticamente")
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}

	reportsDir := expandHomePath(*dir)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", *port)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := ensureLocalDaemon(ctx, *port, reportsDir); err != nil {
		return err
	}

	targetURL := baseURL + "/"

	if fileArg != "" {
		rawBytes, err := os.ReadFile(fileArg)
		if err != nil {
			return fmt.Errorf("leyendo archivo %s: %w", fileArg, err)
		}
		mdContent := string(rawBytes)
		fm := render.ParseFrontmatter(mdContent)
		title := titleFromMarkdown(mdContent, fm.Title, fileArg)
		proj := strings.TrimSpace(*project)
		if proj == "" {
			proj = fm.Project
		}
		if proj == "" {
			proj = filepath.Base(filepath.Dir(mustAbs(fileArg)))
		}

		bridge := &mcpserver.HTTPBridgeBackend{BaseURL: baseURL}
		pubOut, err := bridge.PublishReport(ctx, mcpserver.PublishReportInput{
			IdempotencyKey:  "file:" + filepath.ToSlash(mustAbs(fileArg)),
			Title:           title,
			Project:         proj,
			Markdown:        mdContent,
			InteractiveMode: false,
		})
		if err != nil {
			return err
		}
		targetURL = pubOut.URL
		fmt.Printf("Reporte publicado: %s", pubOut.ID)
		if pubOut.Action != "" {
			fmt.Printf(" (%s · rev %d)", pubOut.Action, max(pubOut.Revision, 1))
		}
		fmt.Println()
		if pubOut.ChangeNote != "" {
			fmt.Printf("Cambio aplicado:   %s\n", pubOut.ChangeNote)
		}
		fmt.Printf("Fuente Markdown:   %s\n", pubOut.MarkdownPath)
		fmt.Printf("URL del visor:     %s\n", targetURL)
		if pubOut.Cost != nil && pubOut.Cost.ConsoleLine != "" {
			fmt.Printf("Costo estimado:    %s\n", pubOut.Cost.ConsoleLine)
		}
		if len(pubOut.Warnings) > 0 {
			fmt.Printf("Avisos ADR 1:      %d\n", len(pubOut.Warnings))
		}
	} else {
		fmt.Printf("Visor activo en:   %s\n", targetURL)
	}

	if !*noBrowser {
		_ = openBrowserURL(targetURL)
	}
	return nil
}

func ensureLocalDaemon(ctx context.Context, port int, reportsDir string) error {
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/api/health", port)
	if checkDaemonAlive(ctx, healthURL) {
		return nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolviendo ruta del binario dioptra: %w", err)
	}

	indexDir := filepath.Join(reportsDir, ".index")
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return err
	}
	logFile, _ := os.OpenFile(filepath.Join(indexDir, "daemon.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)

	cmd := exec.Command(exePath, "serve", "--port", fmt.Sprintf("%d", port), "--dir", reportsDir)
	configureDetachedDaemon(cmd)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return fmt.Errorf("iniciando demonio dioptra serve en segundo plano: %w", err)
	}
	_ = cmd.Process.Release()
	if logFile != nil {
		_ = logFile.Close()
	}

	// Margen amplio en Windows corporativo por si EDR/Defender analiza el binario en arranque en frío
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if checkDaemonAlive(ctx, healthURL) {
			return nil
		}
		time.Sleep(80 * time.Millisecond)
	}
	return fmt.Errorf("tiempo agotado esperando a que dioptra serve responda en %s", healthURL)
}

func checkDaemonAlive(ctx context.Context, healthURL string) bool {
	cCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(cCtx, http.MethodGet, healthURL, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func openBrowserURL(urlStr string) error {
	if !strings.HasPrefix(urlStr, "http://127.0.0.1:") {
		return fmt.Errorf("URL bloqueada fuera de loopback local: %s", urlStr)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", urlStr)
	case "darwin":
		cmd = exec.Command("open", urlStr)
	default:
		cmd = exec.Command("xdg-open", urlStr)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func mustAbs(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// isAddrInUse detecta "puerto ocupado" de forma portable. En Windows el errno es WSAEADDRINUSE (10048)
// y el texto está localizado, por lo que además de errors.Is se revisa el errno numérico.
func isAddrInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return uintptr(errno) == 10048 || uintptr(errno) == 98 || uintptr(errno) == 48
	}
	return false
}

// dioptraAlreadyServing confirma que quien ocupa el puerto es realmente Dioptra (no otro programa)
// consultando /api/health con un timeout corto.
func dioptraAlreadyServing(baseURL string) bool {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Get(baseURL + "/api/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	buf := make([]byte, 512)
	n, _ := resp.Body.Read(buf)
	return strings.Contains(string(buf[:n]), `"service":"dioptra"`)
}
