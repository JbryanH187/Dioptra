package store

import (
	"context"
	"testing"
)

func TestWorkspaceGroupsAndAudit(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	ctx := context.Background()

	publish := func(project, title string) {
		t.Helper()
		_, err := st.Publish(ctx, PublishParams{
			Project:  project,
			Title:    title,
			Markdown: "# " + title + "\n\nContenido de prueba.",
		})
		if err != nil {
			t.Fatalf("Publish(%s): %v", project, err)
		}
	}

	publish("SnapProof", "Auditoría 1")
	publish("SnapProof", "Auditoría 2")
	publish("snapshooter", "Guía T6")
	publish("genius-shell", "Postmortem")
	publish("GeniusShell", "Update 5.4")
	publish("dioptra", "Tablero")
	publish("DioptraEngine", "Update rc3")
	publish("tsugi", "Brecha v3")
	publish("tsugi-backend", "Tablero backend")
	publish("tsugi-mobile", "Tablero mobile")
	publish("tsugi-ux", "Tablero ux")

	audit, err := st.AuditWorkspaces(ctx, nil)
	if err != nil {
		t.Fatalf("AuditWorkspaces: %v", err)
	}

	// Debe detectar los 3 sub-equipos bajo tsugi.
	foundSub := map[string]string{}
	for _, s := range audit.Subteams {
		foundSub[s.Project] = s.Parent + ":" + s.Label
	}
	for _, want := range []struct{ proj, val string }{
		{"tsugi-backend", "tsugi:backend"},
		{"tsugi-mobile", "tsugi:mobile"},
		{"tsugi-ux", "tsugi:ux"},
	} {
		if foundSub[want.proj] != want.val {
			t.Fatalf("esperaba sub-equipo %s=%q, obtuvo %q (todos: %+v)", want.proj, want.val, foundSub[want.proj], audit.Subteams)
		}
	}

	// Debe detectar duplicados reales: snapproof↔snapshooter, genius-shell↔geniusshell, dioptra↔dioptraengine.
	hasDup := func(a, b string) bool {
		for _, d := range audit.Duplicates {
			if (d.A == a && d.B == b) || (d.A == b && d.B == a) {
				return true
			}
		}
		return false
	}
	if !hasDup("snapproof", "snapshooter") {
		t.Fatalf("faltó duplicado snapproof↔snapshooter en %+v", audit.Duplicates)
	}
	if !hasDup("genius-shell", "geniusshell") {
		t.Fatalf("faltó duplicado genius-shell↔geniusshell en %+v", audit.Duplicates)
	}
	if !hasDup("dioptra", "dioptraengine") {
		t.Fatalf("faltó duplicado dioptra↔dioptraengine en %+v", audit.Duplicates)
	}
	// Sub-equipos de tsugi NO deben aparecer como duplicados entre sí ni contra tsugi.
	if hasDup("tsugi", "tsugi-backend") || hasDup("tsugi-backend", "tsugi-mobile") {
		t.Fatalf("sub-equipos tsugi no debían sugerirse como duplicados: %+v", audit.Duplicates)
	}

	// Agrupar sub-equipo y verificar que se refleja en ListProjects y ParentWorkspaceOf.
	g, err := st.SetWorkspaceGroup(ctx, "tsugi-backend", "tsugi", "")
	if err != nil {
		t.Fatalf("SetWorkspaceGroup: %v", err)
	}
	if g.Label != "backend" || g.ParentKey != "tsugi" {
		t.Fatalf("grupo inesperado: %+v", g)
	}
	if got := st.ParentWorkspaceOf(ctx, "tsugi-backend"); got != "tsugi" {
		t.Fatalf("ParentWorkspaceOf esperaba tsugi, obtuvo %q", got)
	}
	// Debe impedir cadenas A → B → C.
	if _, err := st.SetWorkspaceGroup(ctx, "tsugi-mobile", "tsugi-backend", ""); err == nil {
		t.Fatal("esperaba rechazo al encadenar sub-equipo sobre otro sub-equipo")
	}

	// Ignorar un par debe quitarlo de futuras auditorías.
	if _, err := st.IgnoreWorkspacePair(ctx, "dioptra", "dioptraengine"); err != nil {
		t.Fatalf("IgnoreWorkspacePair: %v", err)
	}
	audit2, err := st.AuditWorkspaces(ctx, nil)
	if err != nil {
		t.Fatalf("AuditWorkspaces #2: %v", err)
	}
	for _, d := range audit2.Duplicates {
		if (d.A == "dioptra" && d.B == "dioptraengine") || (d.A == "dioptraengine" && d.B == "dioptra") {
			t.Fatalf("par ignorado no debía reaparecer: %+v", d)
		}
	}
	for _, s := range audit2.Subteams {
		if s.Project == "tsugi-backend" {
			t.Fatalf("sub-equipo ya agrupado no debía reaparecer: %+v", s)
		}
	}

	// ListProjects debe incluir parentKey y subLabel.
	projs, err := st.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	foundGrouped := false
	for _, p := range projs {
		if p.Key == "tsugi-backend" && p.ParentKey == "tsugi" && p.SubLabel == "backend" {
			foundGrouped = true
		}
	}
	if !foundGrouped {
		t.Fatalf("ListProjects no expuso parentKey/subLabel de tsugi-backend: %+v", projs)
	}
}
