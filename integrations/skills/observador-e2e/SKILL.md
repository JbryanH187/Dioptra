---
name: observador-e2e
description: Observe and document an end-to-end test that the USER performs (any system, any stack) and publish a live report in Dioptra by test case, with services, URLs, environments, ports, requests/responses, validations, inter-service calls, logs and log-quality gaps. Use when the user says they are going to test a flow ("voy a probar", "prueba e2e", "flujo de punta a punta", "end to end"), starts services to test something, or asks for observability of a flow. Requires the Dioptra MCP server (tools e2e_start, e2e_record, e2e_ingest_har, e2e_finish).
---

# Observador E2E

You **observe and document** a test that a person performs. You do not drive the test, click for the user or change code. The goal is to give the team observability of a complete flow:
- what happened in each step;
- which services were involved;
- what each one answered;
- what the logs show;
- which log is missing to diagnose the flow better.

## 1. Offer it, don't impose it

When the user is about to test a flow, ask **once**:

> ¿Quieres que genere el reporte end-to-end en Dioptra para tener mayor observabilidad?

Use a choice question if your client has one, with these options:
- "Sí, documentar la prueba" (recommended)
- "No por ahora"

If the user says no, do not ask again in this session unless they bring it up.

## 2. Start and map the system (before the user tests)

1. Call `e2e_start` with:
   - `project`: the repository root folder name;
   - `title`: the flow and environment, e.g. "Checkout · QA";
   - `environment`;
   - `agent`: your own name.
2. Map the services from the repository and record each one with `e2e_record` (`kind: "service"`), giving its name, `base_url`, `environment`, `port` and `log_source`.
   - **Config files to read:** `.env*`, `wp-config.php`, `appsettings*.json`, `application*.yml|properties`, `docker-compose*.yml`, `launchSettings.json`, proxy configs, and the constants in front-end code that point to APIs.
   - **Local ports:** when testing on localhost, list the listening ports (`netstat -ano | findstr LISTENING` on Windows, `ss -ltnp` on Linux) to match each service to its port.
   - **Where each log lives:** a file path, a container, or the console of a running process. If an API's log is not reachable, say so in `log_source` and use the logs of the application that calls it.
3. Tell the user, in 3-4 lines:
   - the run is on, with its link;
   - which services you found and which logs you can read;
   - how to hand you the evidence: say "listo" at the end of each case, and if possible export the HAR (DevTools › Network › right click › *Save all as HAR*) and say where it is.

## 3. During the test: one case per scenario

- Open a case with `kind: "case"`, giving `title` (e.g. "Payment with insufficient funds") and `expected` when you know the expected result.
- When the user says "listo":
  1. **HAR:** call `e2e_ingest_har` with the path the user gives. Dioptra turns every request and response into steps and masks secrets and personal data. It costs you no tokens.
  2. **Logs:** read only the time window of the case (from the case start to "listo") or filter by the case's correlation id. Do not read whole log files. Record the relevant excerpt as a step with `step_kind: "log"`, its `service` and a `verdict`.
  3. **Validations and calls between services** that the HAR does not show (the front end calls A, A calls B): record each as a step with `step_kind: "validation"` or `"request"`, with `from` and `service`, and say in `detail` what was validated and how it resolved.
  4. **Database checks**, if you have read-only access: record them as a step with `step_kind: "query"`, putting the SQL in `request` and the result in `response`.
  5. **Close the case** with `kind: "case"`, its `case` number and `result` (what actually happened).
- If the user tests without a HAR, record the steps they describe, plus what you find in the logs.

## 4. Log quality (the most valuable part for the team)

For each service, check the logs against this rubric. Record each gap with `kind: "finding"`, giving `service`, `severity`, `issue`, `where` and `suggestion`.

| Check | Example of a gap |
|---|---|
| **Correlation** between services (request id, correlation id, trace) | "api-pagos does not log X-Correlation-Id" |
| **Input and output** of each operation | "the call to distribución does not log the amount it sent" |
| **Business-rule outcome** and its reason | "the NSF rejection does not say which rule applied" |
| **Levels** used correctly | "a functional error logged as INFO" |
| **Errors with context** (ids, values, cause), not just "Error" | "catch logs only 'Error al guardar'" |
| **No sensitive data** (tokens, passwords, cards, personal data) | "the log prints the token in plain text" → `severity: "error"` |
| **A log exists** at the critical steps | "no log between validating and sending the payment" |

Make every `suggestion` concrete: say what to log, where, and with which fields.

## 5. Finish

When the user says they are done, call `e2e_finish` with a 3-6 line summary:
- what was tested;
- the result of each case;
- the main risks;
- the top log gaps.

Give the user the link and the counts: cases, steps, services and log improvements.

## Rules

- **Privacy:** Dioptra masks before saving, but never copy into `request`, `response` or `log` more than is needed. Use test data and test environments; if you see production customer data, warn the user.
- **No invention:** every step comes from a HAR, a log, a command output or what the user tells you. If something could not be verified, record it as `verdict: "info"` and say so in `detail`.
- **Read only:** do not edit code or restart services to "make it work" during the observation. If you find a bug, record it as a finding and propose the fix at the end.
- The user can turn the observation off at any time; then call `e2e_finish` with what you have so far.
