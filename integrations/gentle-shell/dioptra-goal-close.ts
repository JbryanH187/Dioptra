// Extensión ligera de DioptraEngine para Gentle Shell / Pi.
// Reglas verificadas contra gentle-shell (extensions/gentle-design.ts & lib/append-system-prompt.ts):
// 1. No inyectar a subagentes trabajadores (GENTLE_PI_AGENTS_CHILD === "1").
// 2. Solo activar cuando Dioptra esté cargado entre las herramientas MCP de Pi.
// 3. Inyectar mediante systemPromptOptions.appendSystemPrompt para que alcance todos los proveedores.

const DIOPTRA_GOAL_CLOSE_RULE = `## Entrega Visual al Cierre (DioptraEngine MCP)
- **Solo al cierre (Paso 7 Close de ODD):** Nunca publiques borradores ni avances durante los pasos 1-6 ni en respuestas menores a 40 líneas.
- **Entregables extensos (> 40 líneas, planes, auditorías, comparativas o especificaciones):** No imprimas el reporte completo en la terminal. Publícalo con \`publish_report\` (\`title\`, \`project\`, \`markdown\`, y opcionalmente \`model\` con tu modelo activo) y responde en terminal únicamente un resumen de 3 a 5 líneas + el \`id\` + la \`url\` **+ la línea literal \`cost.consoleLine\` que devuelve \`publish_report\`** (nunca metas ese costo en la UI).
- **Idempotencia (cero duplicados):** Si modificas un reporte ya publicado, NO crees copias repetidas; reenvía el mismo \`id\` o \`idempotency_key\` (o mismo \`title\`+\`project\`) junto con \`change_note\` (1 línea explicando qué cambió) para actualizar ese mismo documento in-place con leyenda de última actualización.
- **Markdown primero (0 JSON):** envía \`{ title, project, tool: 'Gentle Shell', model, markdown }\` (project = nombre de la carpeta raíz del repo). El Markdown de GitHub ya se convierte en bloques ricos: \`##\` → secciones, tablas → tablas ordenables, \`> [!WARNING] Título\` → callout, \`- [ ] tarea @dueño\` → checklist aprobable, fences \`mermaid\` → diagramas, fence \`kpi\` (\`Etiqueta | valor | unidad | delta | sev\`) → KPIs, fence \`diff\` → diff resaltado, \`<details><summary>\` → sección plegada. Sigue la skill \`dioptra-report-writer\` para la estructura.
- **Solo si el Markdown no alcanza** (selección de KPIs a medida, columnas tipadas, simulación en \`free-canvas\`): llama \`get_layout_contract\` (con \`include_canvas_tokens: true\` solo para canvas) y envía \`blocks_override\` junto con el \`markdown\` completo.
- **Checklists:** Si el humano ya revisó un reporte con casillas pendientes, consulta \`get_report_state\` antes de ejecutar cambios.`;

interface BeforeAgentStartEvent {
	systemPromptOptions?: {
		appendSystemPrompt?: string;
	} | null;
}

interface ExtensionAPI {
	getAllTools?: () => Array<{ name: string }>;
	on(
		event: "before_agent_start",
		handler: (event: BeforeAgentStartEvent) => undefined,
	): void;
}

function appendSystemPromptOnce(
	options: BeforeAgentStartEvent["systemPromptOptions"],
	text: string,
): void {
	if (!options || !text) return;
	const current = options.appendSystemPrompt ?? "";
	if (current.includes(text)) return;
	options.appendSystemPrompt = current ? `${current}\n\n${text}` : text;
}

export default function dioptraGoalClose(pi: ExtensionAPI): void {
	pi.on("before_agent_start", (event) => {
		if (process.env.GENTLE_PI_AGENTS_CHILD === "1") {
			return undefined;
		}
		const tools = pi.getAllTools?.() ?? [];
		const hasDioptra = tools.some(
			(t) =>
				t.name === "publish_report" ||
				t.name.endsWith("__publish_report") ||
				t.name.startsWith("mcp__dioptra__"),
		);
		if (hasDioptra) {
			appendSystemPromptOnce(event.systemPromptOptions, DIOPTRA_GOAL_CLOSE_RULE);
		}
		return undefined;
	});
}
