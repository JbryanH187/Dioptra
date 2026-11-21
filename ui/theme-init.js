// Aplica el tema ANTES del primer pintado (script clásico y síncrono en <head>):
// preferencia guardada → preferencia del sistema → oscuro. Evita el destello de
// tema incorrecto. Archivo externo porque la CSP no permite scripts inline.
(function () {
  var theme = 'dark';
  try {
    var saved = localStorage.getItem('dx-theme');
    if (saved === 'light' || saved === 'dark') {
      theme = saved;
    } else if (window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches) {
      theme = 'light';
    }
  } catch (_) {
    // localStorage deshabilitado: se usa el valor por defecto
  }
  document.documentElement.dataset.theme = theme;
})();
