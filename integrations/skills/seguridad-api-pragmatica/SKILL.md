---
name: seguridad-api-pragmatica
description: Pragmatic 3-layer Backend & API Security copilot (1. Auth/RBAC/BOLA-IDOR/JWT, 2. Rate-Limiting & Payload-Bounds OWASP API4, 3. Conditional Anti-Prompt-Injection OWASP LLM01/LLM06). Based on MIT/CC0 patterns (addyosmani, kodustech, VoltAgent) with zero compliance bloat.
---

# Seguridad API Pragmática (Copiloto de 3 Capas)

Apoya al agente de Backend endureciendo endpoints **por capas y solo con lo indispensable** (bases MIT/CC0: `addyosmani/security-and-hardening`, `kodustech/auditing-access-control`, `VoltAgent/security-auditor` y OWASP API/LLM Top 10).

> **Anti-alucinación:** Prohibido inventar burocracia (SOC2, ISO, HIPAA, WAF, HSM) o auditar IA donde no hay LLMs. Cada punto exige `archivo:línea` real y fix mínimo en el stack actual.

## Las 3 Capas (solo las que toquen al cambio)

### Capa 1 · Identidad y Control de Acceso (OWASP API1/API2/API5)
1. **Rutas públicas explícitas:** nada entra en `permitAll()` por accidente; revisar cobertura por verbo HTTP (`GET` público ≠ `POST/PUT/PATCH/DELETE` público).
2. **Cero IDOR / BOLA:** estar logueado no basta. Toda consulta/mutación valida propiedad en SQL/RLS (`business_id = tenancy.current_tenant()` con `FORCE RLS` o `user_id` del token), nunca confiando en IDs del body/query.
3. **Cero Mass Assignment:** recibir DTOs/`record` explícitos; nunca enlazar JSON directo a entidades ni aceptar campos de rol/precio/saldo no autorizados.
4. **JWT / OAuth sano:** algoritmo fijo en lista blanca (rechazar `alg: none`), expiración corta + refresh rotativo, secretos solo en env y cualquier login de prueba aislado con `@Profile("dev")` (401/404 fuera de `dev`).

### Capa 2 · Disponibilidad y Anti-Sobrecarga (OWASP API4)
1. **Rate Limiting proporcional:**
   - Auth/OTP: `~10 req / 15 min` por IP.
   - Escrituras/reservas: `~30 req / min` por usuario o IP.
   - Lecturas públicas: `~120 req / min` por IP.
   - Responder `429` + `Retry-After` (en memoria si es monolito de 1 nodo; en Redis si escala horizontalmente).
2. **Cotas duras:** cuerpo HTTP acotado (ej. `64 KB` JSON), `@Valid` + `@Size`/`@Max` en DTOs, `LIMIT <= 100` obligatorio en listados y timeouts en BD/HTTP (`<= 10–15 s`).

### Capa 3 · Anti-Prompt Injection (CONDICIONAL — OWASP LLM01/LLM06)
- **Sin LLM en el endpoint:** marca **`Capa 3: N/A (sin LLM)`** y omítela.
- **Con LLM:** (1) delimitadores estrictos entre prompt de sistema y datos externos no confiables; (2) salida validada con esquema JSON cerrado; (3) prohibido ejecutar mutaciones sensibles (pagos, borrados, permisos) sin validación determinista en código o confirmación humana; (4) tope de caracteres + rate-limit de tokens por usuario.

## Fronteras de decisión
- **Siempre:** SQL parametrizado, secretos fuera de git/logs y pruebas negativas (`401`, `403`, `429` en `.bru` o tests).
- **Preguntar al usuario:** abrir rutas públicas nuevas, cambiar CORS/auth global o relajar rate limits.
- **Nunca:** meter dependencias pesadas sin permiso ni reportar riesgos teóricos sin `archivo:línea`.
