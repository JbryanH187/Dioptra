package ui

import "embed"

// Assets empaqueta exclusivamente los artefactos de producción del visor,
// excluyendo preview.html y preview.js del binario compilado.
//
//go:embed index.html app.js theme-init.js styles/*.css components/*.js fonts/*.woff2 fonts/*.txt vendor/mermaid.min.js
var Assets embed.FS

// TokensCSS expone las variables de ambos temas para inyección directa inline
// en el endpoint aislado /canvas/{docId}/{blockId}.
//
//go:embed styles/tokens.css
var TokensCSS string
