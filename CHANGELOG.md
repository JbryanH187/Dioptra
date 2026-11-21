# Changelog — DioptraEngine

Formato: una sección por versión publicada (`git tag vX.Y.Z`; el binario se compila con
`./scripts/check.ps1 -Version vX.Y.Z`). Lo más reciente arriba.

## v1.1.0 — 2026-11-21

### Tablero de equipo (Dev · Tester · PO)
- Tablero por proyecto con el rol de cada agente y la tarea que le toca.
- **Moderador contra alucinaciones entre agentes:**
  - revisa evidencias (comando, `archivo:línea`, enlace);
  - detecta decisiones en conflicto, rebotes y acaparamiento;
  - solo él usa «verificado» y «alerta».
- La conversación del equipo es visible: decisiones, preguntas abiertas y el reporte «Chat del equipo».
- **Tus aprobaciones:**
  - En modo manual (por defecto), una tarea aprobada por el PO espera tu casilla en el visor.
  - El modo automático solo lo activas tú.
  - Ningún agente puede firmar como el usuario.
- **Menos ruido del moderador:**
  - las preguntas de tareas ya cerradas no alertan;
  - «estancada» solo se marca si el resto del tablero siguió avanzando sin esa tarea.

### Observador E2E (v1)
- Documenta pruebas de punta a punta a partir de un HAR y publica su reporte en vivo.

### CLI
- `dioptra start`: deja el demonio corriendo en segundo plano y termina.
- `dioptra autostart on | off | status`: arranque al iniciar sesión en Windows, sin ventana y sin permisos de administrador. El `doctor` muestra su estado.

### Mantenimiento
- `store.go` partido por responsabilidad (F7).
