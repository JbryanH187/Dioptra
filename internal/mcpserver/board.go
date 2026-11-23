package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dioptra/internal/board"
)

// Tablero de equipo: agentes con rol (dev, tester, po) coordinan tareas aquí en vez de
// conversar entre sí. Descripciones en inglés (los modelos las siguen mejor).

// BoardTaskInput crea, toma o mueve una tarea.
type BoardTaskInput struct {
	Action    string   `json:"action" jsonschema:"create | claim | move | comment"`
	Project   string   `json:"project" jsonschema:"Repository root folder name"`
	ID        string   `json:"id,omitempty" jsonschema:"claim/move: task id (e.g. T3)"`
	Title     string   `json:"title,omitempty" jsonschema:"create: short title"`
	Detail    string   `json:"detail,omitempty" jsonschema:"create: what to do"`
	Criteria  []string `json:"criteria,omitempty" jsonschema:"create: acceptance criteria (verifiable)"`
	Role      string   `json:"role,omitempty" jsonschema:"create: dev | tester | po | ux (default dev)"`
	DependsOn []string `json:"depends_on,omitempty" jsonschema:"create: task ids that must be done first"`
	By        string   `json:"by,omitempty" jsonschema:"Who acts, signed with your role: 'dev · genius', 'ux · genius', 'tester · claude', 'moderador · genius'"`
	AsRole    string   `json:"as_role,omitempty" jsonschema:"Your role (dev | tester | po | ux | moderador) if by does not start with it. You may only claim and move tasks of your role"`
	To        string   `json:"to,omitempty" jsonschema:"move: en_curso | en_revision | hecha | rechazada | pendiente"`
	NextRole  string   `json:"next_role,omitempty" jsonschema:"move: role that picks it up next (default: dev→tester on review, back to dev on reject)"`
	Evidence  string   `json:"evidence,omitempty" jsonschema:"move to en_revision/hecha: test command and result, report url, file:line"`
	Reason    string   `json:"reason,omitempty" jsonschema:"move to rechazada: failed criterion and how to reproduce"`
	Tag       string   `json:"tag,omitempty" jsonschema:"comment: decision | pregunta | respuesta | riesgo | acuerdo | nota | verificado | alerta"`
	Text      string   `json:"text,omitempty" jsonschema:"comment: one key message (max 600 chars) another role must know"`
}

// BoardListInput lista tareas.
type BoardListInput struct {
	Project string `json:"project"`
	Role    string `json:"role,omitempty" jsonschema:"Only tasks for this role: dev | tester | po"`
	Status  string `json:"status,omitempty" jsonschema:"pendiente | en_curso | en_revision | rechazada | hecha"`
}

// BoardBackend es opcional: los backends que lo implementan exponen el tablero.
type BoardBackend interface {
	BoardTask(ctx context.Context, in BoardTaskInput) (*board.Result, error)
	BoardList(ctx context.Context, in BoardListInput) (*board.Result, error)
}

// InputToBoard traduce la entrada MCP al dominio.
func InputToBoard(in BoardTaskInput) board.Input {
	return board.Input{
		Action: in.Action, Project: in.Project, ID: in.ID, Title: in.Title, Detail: in.Detail,
		Criteria: in.Criteria, Role: in.Role, DependsOn: in.DependsOn, By: in.By, To: in.To,
		NextRole: in.NextRole, Evidence: in.Evidence, Reason: in.Reason, Tag: in.Tag, Text: in.Text, AsRole: in.AsRole,
	}
}

func registerBoardTools(srv *mcp.Server, b BoardBackend, readOnly, notDestructive *mcp.ToolAnnotations) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "board_list",
		Description: "Team board (dev · tester · po): list tasks of a project, optionally for your role and a status. " +
			"Start every work cycle here: pick tasks for your role, respecting depends_on. Also returns alerts from the board audit (unverifiable or nonexistent evidence, conflicting decisions, ping-pong, unanswered questions): the moderador verifies them.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in BoardListInput) (*mcp.CallToolResult, board.Result, error) {
		return boardReply(b.BoardList(ctx, in))
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name: "board_task",
		Description: "Team board: create a task (with verifiable acceptance criteria), claim one for your role, move it, or comment on it. " +
			"Flow: pendiente → en_curso (claim) → en_revision (needs evidence; goes to tester) → hecha (needs evidence) or rechazada (needs reason; back to dev). " +
			"Closing: in manual mode (default) the PO's move to hecha waits for the USER, who approves on the Dioptra board; never sign as the user. " +
			"comment {id, tag, text}: leave KEY messages only (a decision, a question for another role, its answer, a risk, an agreement), not chatter. " +
			"The board is published live in Dioptra (decisions, open questions and the team conversation); agents coordinate through it instead of chatting.",
		Annotations: notDestructive,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in BoardTaskInput) (*mcp.CallToolResult, board.Result, error) {
		return boardReply(b.BoardTask(ctx, in))
	})
}

func boardReply(res *board.Result, err error) (*mcp.CallToolResult, board.Result, error) {
	if err != nil {
		return nil, board.Result{}, err
	}
	return nil, *res, nil
}

// --- Puente HTTP ---

func (b *HTTPBridgeBackend) boardCall(ctx context.Context, method, path string, in any) (*board.Result, error) {
	if b.EnsureDaemon != nil {
		if err := b.EnsureDaemon(ctx); err != nil {
			return nil, err
		}
	}
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(b.BaseURL, "/")+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return nil, fmt.Errorf("%s", e.Error)
		}
		return nil, fmt.Errorf("error HTTP %d desde demonio dioptra", resp.StatusCode)
	}
	var out board.Result
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (b *HTTPBridgeBackend) BoardTask(ctx context.Context, in BoardTaskInput) (*board.Result, error) {
	return b.boardCall(ctx, http.MethodPost, "/api/board/task", in)
}

func (b *HTTPBridgeBackend) BoardList(ctx context.Context, in BoardListInput) (*board.Result, error) {
	q := url.Values{"project": {in.Project}}
	if in.Role != "" {
		q.Set("role", in.Role)
	}
	if in.Status != "" {
		q.Set("status", in.Status)
	}
	return b.boardCall(ctx, http.MethodGet, "/api/board?"+q.Encode(), nil)
}
