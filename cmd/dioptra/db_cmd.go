package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"dioptra/internal/roots"
	"dioptra/internal/store"
)

// runDB agrupa el mantenimiento acotado del índice SQLite. No acepta SQL arbitrario.
//
//	dioptra db check   [--json]                 solo lectura
//	dioptra db backup  [--out ruta.db]          copia consistente (VACUUM INTO) + quick_check
//	dioptra db reindex                          reconstruye reports_search (escritura)
//	dioptra db alias   list|set <alias> <canónica>|remove <alias>
//	dioptra db group   list|set <proyecto> <padre> [etiqueta]|remove <proyecto>
//	dioptra db audit   [--json]                 detecta duplicados y sub-equipos sin modificar nada
func runDB(args []string) error {
	if len(args) == 0 {
		return errors.New("uso: dioptra db <check|backup|reindex|alias|group|audit> [opciones]")
	}
	sub, rest := args[0], args[1:]
	fs := flag.NewFlagSet("db "+sub, flag.ContinueOnError)
	dir := fs.String("dir", store.DefaultReportsDir(), "Directorio raíz de reportes Markdown")
	asJSON := fs.Bool("json", false, "Salida JSON")
	out := fs.String("out", "", "Destino del respaldo (por defecto <dir>/.backups/dioptra-<fecha>.db)")
	// Go detiene el parseo en el primer posicional: se reparsea el resto para admitir flags en cualquier posición.
	var positional []string
	for {
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	st, err := store.Open(expandHomePath(*dir))
	if err != nil {
		return err
	}
	defer st.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	switch sub {
	case "check":
		r, err := st.CheckDB(ctx)
		if err != nil {
			return err
		}
		if *asJSON {
			return json.NewEncoder(os.Stdout).Encode(r)
		}
		fmt.Printf("DB: %s\n  esquema v%d (última v%d) · journal=%s · quick_check=%s\n  reportes=%d · filas de búsqueda=%d · FTS=%v · alias=%d\n",
			r.DBPath, r.SchemaVersion, r.LatestVersion, r.JournalMode, r.QuickCheck, r.Reports, r.SearchRows, r.FTS, r.Aliases)
		if len(r.MissingMD) > 0 {
			fmt.Printf("  ⚠ %d fila(s) sin archivo .md:\n", len(r.MissingMD))
			for _, m := range r.MissingMD {
				fmt.Println("    -", m)
			}
		}
		if r.QuickCheck != "ok" || len(r.MissingMD) > 0 || r.SchemaVersion != r.LatestVersion {
			return errors.New("se detectaron hallazgos")
		}
	case "backup":
		dest := *out
		if dest == "" {
			dest = filepath.Join(st.BaseDir(), ".backups", "dioptra-"+time.Now().Format("20060102-150405")+".db")
		}
		if err := st.BackupTo(ctx, dest); err != nil {
			return err
		}
		fmt.Println("Respaldo verificado (quick_check ok):", dest)
	case "reindex":
		n, err := st.RebuildSearchIndex(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("Índice de búsqueda reconstruido: %d documento(s)\n", n)
	case "alias":
		pos := positional
		if len(pos) == 0 {
			pos = []string{"list"}
		}
		switch pos[0] {
		case "list":
			al, err := st.ListProjectAliases(ctx)
			if err != nil {
				return err
			}
			if len(al) == 0 {
				fmt.Println("Sin alias definidos.")
			}
			for _, a := range al {
				fmt.Printf("  %s → %s\n", a.AliasKey, a.CanonicalKey)
			}
		case "set":
			if len(pos) != 3 {
				return errors.New("uso: dioptra db alias set <alias> <canónica>")
			}
			a, err := st.SetProjectAlias(ctx, pos[1], pos[2])
			if err != nil {
				return err
			}
			fmt.Printf("Alias creado: %s → %s\n", a.AliasKey, a.CanonicalKey)
		case "remove":
			if len(pos) != 2 {
				return errors.New("uso: dioptra db alias remove <alias>")
			}
			ok, err := st.RemoveProjectAlias(ctx, pos[1])
			if err != nil {
				return err
			}
			fmt.Println("Alias retirado:", ok)
		default:
			return fmt.Errorf("subcomando alias desconocido: %q", pos[0])
		}
	case "group":
		pos := positional
		if len(pos) == 0 {
			pos = []string{"list"}
		}
		switch pos[0] {
		case "list":
			gs, err := st.ListWorkspaceGroups(ctx)
			if err != nil {
				return err
			}
			if len(gs) == 0 {
				fmt.Println("Sin grupos de sub-equipos definidos.")
			}
			for _, g := range gs {
				fmt.Printf("  %s → %s (%s)\n", g.ProjectKey, g.ParentKey, g.Label)
			}
		case "set":
			if len(pos) < 3 || len(pos) > 4 {
				return errors.New("uso: dioptra db group set <proyecto> <padre> [etiqueta]")
			}
			lbl := ""
			if len(pos) == 4 {
				lbl = pos[3]
			}
			g, err := st.SetWorkspaceGroup(ctx, pos[1], pos[2], lbl)
			if err != nil {
				return err
			}
			fmt.Printf("Sub-equipo agrupado: %s → %s (%s)\n", g.ProjectKey, g.ParentKey, g.Label)
		case "remove":
			if len(pos) != 2 {
				return errors.New("uso: dioptra db group remove <proyecto>")
			}
			ok, err := st.RemoveWorkspaceGroup(ctx, pos[1])
			if err != nil {
				return err
			}
			fmt.Println("Sub-equipo desagrupado:", ok)
		default:
			return fmt.Errorf("subcomando group desconocido: %q", pos[0])
		}
	case "audit":
		rootMap := map[string]string{}
		if reg, err := roots.Load(st.BaseDir()); err == nil && reg != nil {
			for _, e := range reg.List() {
				rootMap[e.Project] = e.Path
			}
		}
		res, err := st.AuditWorkspaces(ctx, rootMap)
		if err != nil {
			return err
		}
		if *asJSON {
			return json.NewEncoder(os.Stdout).Encode(res)
		}
		fmt.Printf("Workspaces analizados: %d · duplicados sugeridos: %d · sub-equipos sugeridos: %d\n",
			res.Projects, len(res.Duplicates), len(res.Subteams))
		if len(res.Duplicates) > 0 {
			fmt.Println("\nPosibles duplicados (usa `dioptra db alias set <alias> <canónica>`):")
			for _, d := range res.Duplicates {
				fmt.Printf("  • %s (%d) ↔ %s (%d) [%.0f%%] — %s · sugerido: %s\n",
					d.A, d.AReports, d.B, d.BReports, d.Score*100, d.Reason, d.Suggestion)
			}
		}
		if len(res.Subteams) > 0 {
			fmt.Println("\nPosibles sub-equipos (usa `dioptra db group set <proyecto> <padre>`):")
			for _, s := range res.Subteams {
				fmt.Printf("  • %s (%d) → %s [%s] — %s\n", s.Project, s.Reports, s.Parent, s.Label, s.Reason)
			}
		}
		if len(res.Duplicates) == 0 && len(res.Subteams) == 0 {
			fmt.Println("Sin duplicados ni sub-equipos pendientes.")
		}
	default:
		return fmt.Errorf("subcomando db desconocido: %q", sub)
	}
	return nil
}
