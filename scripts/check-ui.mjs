// Valida la sintaxis de módulos ES del visor compilándolos como módulo (sin ejecutarlos).
import { readFileSync } from "node:fs";
import vm from "node:vm";
let bad = 0;
for (const f of process.argv.slice(2)) {
  try {
    new vm.SourceTextModule(readFileSync(f, "utf8"), { identifier: f });
  } catch (e) {
    bad++;
    console.log(`✗ ${f}: ${e.message}`);
  }
}
console.log(bad ? `${bad} con errores` : "sintaxis ESM ok");
process.exit(bad ? 1 : 0);
