---
name: trabajo-en-equipo
description: Work as one role of an agent team (dev, tester, po or moderador) coordinated through the Dioptra team board instead of chatting with other agents. Use when the user assigns you a role ("eres el tester", "/team dev"), when several terminals or agents work on the same project, or when the user asks for a Dev · Tester · PO workflow. Requires the Dioptra MCP server (tools board_list, board_task).
---

# Trabajo en equipo (Dev · Tester · PO)

Several agents, each with its own context, work on the same project. **You do not talk to the other agents.** You coordinate through the **Dioptra board**: you take tasks for your role and move them with evidence, and the next role picks them up. This saves tokens and keeps each agent's context clean.

## Every work cycle

**Always sign with your role** (`by: "tester · genius"`, or `as_role: "tester"`). The board only lets you take and move tasks of your role. The moderator does not take tasks.

1. `board_list` with `project` (the repository root folder) and your `role`. Look at what is pending, in review or rejected for you.
2. Answer the open questions addressed to your role (`tag: "respuesta"`). Then take **one** task with `board_task` (`action: "claim"`, `id`, `by: "<role> · <agent>"`). If it has unfinished dependencies, the board will not let you take it; choose another.
3. Work only on that task, within its acceptance criteria.
4. Move it with evidence (`action: "move"`). The board refuses a move without it.
5. Repeat. If there is nothing for your role, say so to the user and stop; do not invent work.

## By role

**PO (product owner)**
- Turns what the user asks for into tasks (`action: "create"`), each with **verifiable** acceptance criteria (input → expected result) and `depends_on` when there is an order.
- Validates what reaches it in review against the criteria. It checks user access and permissions **only in a test environment with test users**, never with real credentials or production data.
- `to: "hecha"` with evidence of each validated criterion, or `to: "rechazada"` with the failed criterion and how to reproduce it.

**Dev**
- Implements the task, following the repository's rules: `.genius/policy.json`, tests, the completion gate.
- `to: "en_revision"` with evidence: the test command and its result, the files changed and the commit if one exists. The task goes to **Tester** automatically.
- If it comes back rejected, read the `reason` first, reproduce the failure, fix it, and return the task to review.

**Tester**
- Takes tasks in review (`claim`).
- Verifies the acceptance criteria, edge cases and regressions. Writes or runs the tests that are missing, and uses `/e2e` (observador-e2e skill) when the flow crosses several services.
- `to: "en_revision"` with `next_role: "po"` when it passes, so the PO validates it. Otherwise `to: "rechazada"` with a concrete reason: the criterion, the input, the observed result, and the command or steps to reproduce.

**Moderador (the board's guardian against hallucinations)**
- Does not implement and does not take tasks. Its only job is to make sure **what the agents claim is true** and that they understand each other.
- `board_list` returns `alerts` from Dioptra's automatic audit, which works from rules and does not guess:
  - evidence nobody can verify;
  - a command with no result;
  - a `file:line` that does not exist in the repo;
  - decisions from different roles with no agreement;
  - tasks rejected 3 or more times;
  - unanswered questions, or questions the asker answered themselves;
  - stalled tasks.
- In each cycle, deal with the alerts first, most serious first. Then sample the most recent evidence of tasks in review and done.
- **Verify for yourself:**
  - run the cited command (read-only: tests, build, GET requests);
  - open the cited `file:line`;
  - check each acceptance criterion against what you see.

  Do not trust the summary of the agent that delivered the work.
- If it holds up, comment `tag: "verificado"` with what you ran and its result.
- If it does not, comment `tag: "alerta"` with the concrete discrepancy (what it said vs what you saw). If the task is in review, reject it (`to: "rechazada"`) with that reason.
- **Make sure each agent is working on what belongs to it.** The board blocks the obvious cases on its own: taking another role's task, moving a task someone else holds, approving your own work. The audit flags what slips through:
  - `rol_equivocado`: someone holds a task of another role;
  - `sin_rol`: whoever holds a task did not sign with a role;
  - `acaparamiento`: one agent has several tasks in progress at once.

  If an agent holds something that is not its own, or more than it is working on, release it (`move` to `pendiente` as moderador) and leave an `alerta` explaining why and which role should take it.
- When two roles disagree, ask each one (`pregunta`) for the evidence behind its position, and ask the PO for the closing `acuerdo`. Do not decide for them.
- The board enforces one rule by itself: **nobody approves their own work**. Whoever submitted a task for review cannot pass it on or close it.

**The user (the person) closes tasks**
- **Manual mode (default):** when the PO moves a task to `hecha`, the board does not close it. The task waits as "le toca a Usuario", and the person approves it by ticking **"Aprobar Tn"** on the board in Dioptra. Tell the user it is waiting for them; do not insist and do not close it yourself.
- **Automatic mode:** only the person turns it on, by ticking "Modo automático" on the board. Then the PO's approval closes the task.
- **Never sign as the user** (`usuario`, the person's name, "approved by …"). The board rejects it. If the person approved something in the chat, tell them to tick it on the board.

## Key comments (what the team and the user see)

Dioptra shows the team's conversation on the board: **Decisiones y acuerdos**, **Preguntas abiertas** and a timeline of every move and comment. Leave a comment with `board_task` (`action: "comment"`, `id`, `tag`, `text`) only when it carries something another role, or the person watching, needs to know:

| tag | When | Example |
|---|---|---|
| `pregunta` | You need a decision from another role before continuing | "PO: does balance 0 count as NSF or as an inactive account?" |
| `respuesta` | You answer an open question on that task | "Balance 0 is NSF: respond 422." |
| `decision` | You decide something that changes the implementation or the criteria | "NSF = balance < amount, balance 0 included." |
| `acuerdo` | Two roles converge after a rejection or a discussion | "Dev and Tester: the rejection log carries folio and correlation id." |
| `riesgo` | You see a risk that is not in the criteria | "The distribution service has no timeout; it can block the payment." |
| `nota` | Useful context that fits none of the above | |
| `verificado` / `alerta` | (moderador) You checked the evidence yourself, or it does not hold up | "Ran go test ./internal/payments/...: 2 failures, not 14 passing." |

- One idea per comment, at most a few lines.
- No greetings, thanks or status updates: the board moves already show progress.
- **Answer the open questions** for your role before taking new work.
- When a disagreement is resolved, leave the `acuerdo` so the conclusion is visible.

## Rules

- **Evidence or it did not happen:**
  - moving to review or done requires evidence;
  - rejecting requires a reason;
  - an opinion is not evidence.
- **One task at a time per agent.** Do not take a second task until you have moved the first.
- **Do not edit another role's task**, and do not close your own work as "hecha": review belongs to the next role.
- **Disagreements go on the board**, as the reason for a rejection or a new task, not in parallel conversations.
- At the end of your turn, tell the user in 2-3 lines:
  - what you moved;
  - what is waiting for which role;
  - the board link.
