// ==========================================================================
// DioptraEngine Viewer — ui/components/index.js
// Punto único de registro del Catálogo Atómico Cerrado (9 componentes) + Shell
// ==========================================================================

import './dioptra-sigil.js';

export { normalizeReportEnvelope, normalizeBlock } from './shared.js';
export { DxKpiGrid } from './kpi-grid.js';
export { DxSeverityCallout } from './severity-callout.js';
export { DxDataTable } from './data-table.js';
export { DxCollapsibleSection } from './collapsible-section.js';
export { DxCodeView } from './code-view.js';
export { DxDiagramBlock } from './diagram-block.js';
export { DxActionChecklist } from './action-checklist.js';
export { DxProseBlock } from './prose-block.js';
export { DxFreeCanvas } from './free-canvas.js';
export { DxTimeline } from './dx-timeline.js';
export { DxSessions } from './dx-sessions.js';
export { DxReport, instantiateBlockTree } from './dx-report.js';
export { DxApp } from './dx-app.js';
export { DxEvidence } from './dx-evidence.js';
import './dx-tooltip.js';
