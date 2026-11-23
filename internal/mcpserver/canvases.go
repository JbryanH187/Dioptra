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

	"dioptra/internal/store"
)

// CanvasRegisterInput registra o actualiza un lienzo multipantalla local opt-in.
type CanvasRegisterInput struct {
	Project   string `json:"project" jsonschema:"Repository/workspace name (e.g. tsugi)"`
	CanvasID  string `json:"canvas_id,omitempty" jsonschema:"Stable canvas slug (defaults to 'default' or project slug)"`
	Title     string `json:"title,omitempty" jsonschema:"Human-friendly canvas title"`
	DirPath   string `json:"dir_path" jsonschema:"Absolute local directory containing canvas.json (or project/canvas.json) and screen HTML files"`
	EntryJSON string `json:"entry_json,omitempty" jsonschema:"Relative path to manifest JSON inside dir_path (auto-detects project/canvas.json or canvas.json)"`
}

// CanvasRegisterOutput devuelve el lienzo registrado y su URL directa en el visor.
type CanvasRegisterOutput struct {
	OK     bool               `json:"ok"`
	Canvas store.DesignCanvas `json:"canvas"`
	URL    string             `json:"url"`
}

// CanvasListInput lista lienzos opt-in registrados.
type CanvasListInput struct {
	Project string `json:"project,omitempty" jsonschema:"Optional project filter (resolves aliases and parent workspaces)"`
}

// CanvasListOutput devuelve los lienzos registrados.
type CanvasListOutput struct {
	Count    int                  `json:"count"`
	Canvases []store.DesignCanvas `json:"canvases"`
}

// CanvasFeedbackInput consulta comentarios y marcas humanas sobre un lienzo.
type CanvasFeedbackInput struct {
	Project   string `json:"project" jsonschema:"Project name (resolves aliases and parent workspace)"`
	CanvasID  string `json:"canvas_id,omitempty" jsonschema:"Optional canvas_id; omit to read across all canvases of the project"`
	Status    string `json:"status,omitempty" jsonschema:"open (default) | applied | resolved | all"`
	BoardFile string `json:"board_file,omitempty" jsonschema:"Optional screen filename filter (e.g. ExpressLimpio.dc.html)"`
}

// CanvasFeedbackOutput entrega las anotaciones con un resumen compacto listo para editar.
type CanvasFeedbackOutput struct {
	Project     string                   `json:"project"`
	CanvasID    string                   `json:"canvas_id,omitempty"`
	Count       int                      `json:"count"`
	Annotations []store.CanvasAnnotation `json:"annotations"`
	Summary     string                   `json:"summary"`
}

// CanvasResolveInput marca una anotación como aplicada o resuelta tras editar la pantalla.
type CanvasResolveInput struct {
	Project      string `json:"project" jsonschema:"Project name"`
	CanvasID     string `json:"canvas_id" jsonschema:"Canvas id"`
	AnnotationID string `json:"annotation_id" jsonschema:"Annotation id (e.g. ann_a1b2c3d4e5f6)"`
	Status       string `json:"status,omitempty" jsonschema:"applied (default for agents) | resolved | open"`
	Note         string `json:"note" jsonschema:"Concrete summary of what was changed (include file:line)"`
}

// CanvasResolveOutput devuelve la anotación actualizada.
type CanvasResolveOutput struct {
	OK         bool                   `json:"ok"`
	Annotation store.CanvasAnnotation `json:"annotation"`
}

// CanvasBackend es opcional: los backends que lo implementan exponen lienzos opt-in y feedback visual.
type CanvasBackend interface {
	CanvasRegister(ctx context.Context, in CanvasRegisterInput) (*CanvasRegisterOutput, error)
	CanvasList(ctx context.Context, in CanvasListInput) (*CanvasListOutput, error)
	CanvasGetFeedback(ctx context.Context, in CanvasFeedbackInput) (*CanvasFeedbackOutput, error)
	CanvasResolveFeedback(ctx context.Context, in CanvasResolveInput) (*CanvasResolveOutput, error)
}

func registerCanvasTools(srv *mcp.Server, b CanvasBackend, readOnly, notDestructive *mcp.ToolAnnotations) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "canvas_register",
		Description: "Opt-in: register or update a local multi-screen UI/UX design canvas (directory with canvas.json and screen HTML files) " +
			"so the user can inspect it inside Dioptra's modal viewer (Lienzo + Prototipo) and leave visual pin/box/highlight comments.",
		Annotations: notDestructive,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CanvasRegisterInput) (*mcp.CallToolResult, CanvasRegisterOutput, error) {
		res, err := b.CanvasRegister(ctx, in)
		if err != nil {
			return nil, CanvasRegisterOutput{}, err
		}
		return nil, *res, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "canvas_list",
		Description: "List opt-in design canvases registered in Dioptra (optionally filtered by project), including open comment counts.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CanvasListInput) (*mcp.CallToolResult, CanvasListOutput, error) {
		res, err := b.CanvasList(ctx, in)
		if err != nil {
			return nil, CanvasListOutput{}, err
		}
		return nil, *res, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name: "canvas_get_feedback",
		Description: "Read human visual comments/markers (pin, box, highlight) left on a design canvas in Dioptra. " +
			"Returns board_file, normalized 0..1 coordinates, probed DOM selector/text, and exact comment so you can edit the screen accurately.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CanvasFeedbackInput) (*mcp.CallToolResult, CanvasFeedbackOutput, error) {
		if strings.TrimSpace(in.Project) == "" {
			return nil, CanvasFeedbackOutput{}, fmt.Errorf("el parámetro project es obligatorio")
		}
		res, err := b.CanvasGetFeedback(ctx, in)
		if err != nil {
			return nil, CanvasFeedbackOutput{}, err
		}
		return nil, *res, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name: "canvas_resolve_feedback",
		Description: "Mark a canvas annotation as applied (or resolved) after editing the screen file, with a short note citing what changed. " +
			"Triggers live reload of the canvas in the user's browser.",
		Annotations: notDestructive,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CanvasResolveInput) (*mcp.CallToolResult, CanvasResolveOutput, error) {
		if strings.TrimSpace(in.Project) == "" || strings.TrimSpace(in.CanvasID) == "" || strings.TrimSpace(in.AnnotationID) == "" {
			return nil, CanvasResolveOutput{}, fmt.Errorf("project, canvas_id y annotation_id son obligatorios")
		}
		res, err := b.CanvasResolveFeedback(ctx, in)
		if err != nil {
			return nil, CanvasResolveOutput{}, err
		}
		return nil, *res, nil
	})
}

// --- Puente HTTP ---

func (b *HTTPBridgeBackend) canvasHTTP(ctx context.Context, method, path string, in any, out any) error {
	if b.EnsureDaemon != nil {
		if err := b.EnsureDaemon(ctx); err != nil {
			return err
		}
	}
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(b.BaseURL, "/")+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return fmt.Errorf("%s", e.Error)
		}
		return fmt.Errorf("error HTTP %d desde demonio dioptra: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.Unmarshal(data, out)
}

func (b *HTTPBridgeBackend) CanvasRegister(ctx context.Context, in CanvasRegisterInput) (*CanvasRegisterOutput, error) {
	var out CanvasRegisterOutput
	if err := b.canvasHTTP(ctx, http.MethodPost, "/api/canvases/register", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (b *HTTPBridgeBackend) CanvasList(ctx context.Context, in CanvasListInput) (*CanvasListOutput, error) {
	q := url.Values{}
	if in.Project != "" {
		q.Set("project", in.Project)
	}
	path := "/api/canvases"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out CanvasListOutput
	if err := b.canvasHTTP(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (b *HTTPBridgeBackend) CanvasGetFeedback(ctx context.Context, in CanvasFeedbackInput) (*CanvasFeedbackOutput, error) {
	q := url.Values{}
	if in.Status != "" {
		q.Set("status", in.Status)
	}
	if in.BoardFile != "" {
		q.Set("board", in.BoardFile)
	}
	canvasID := in.CanvasID
	if strings.TrimSpace(canvasID) == "" {
		canvasID = "_"
	}
	path := fmt.Sprintf("/api/canvases/%s/%s/annotations", url.PathEscape(in.Project), url.PathEscape(canvasID))
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out CanvasFeedbackOutput
	if err := b.canvasHTTP(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (b *HTTPBridgeBackend) CanvasResolveFeedback(ctx context.Context, in CanvasResolveInput) (*CanvasResolveOutput, error) {
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "applied"
	}
	path := fmt.Sprintf("/api/canvases/%s/%s/annotations/%s",
		url.PathEscape(in.Project), url.PathEscape(in.CanvasID), url.PathEscape(in.AnnotationID))
	var out CanvasResolveOutput
	if err := b.canvasHTTP(ctx, http.MethodPatch, path, map[string]any{
		"status":         status,
		"resolutionNote": in.Note,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
