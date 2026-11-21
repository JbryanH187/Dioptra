package layoutagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultCLITimeout = 180 * time.Second
	maxCLIStdout      = 8 * 1024 * 1024
	maxCLIStderr      = 64 * 1024
)

// resolveCLI valida DIOPTRA_LAYOUT_CLI y localiza el ejecutable en PATH (incluye PATHEXT
// en Windows, p. ej. claude.cmd). Nunca se invoca un intérprete de shell.
func resolveCLI(cliCmd string, explicit bool) (Resolved, error) {
	if cliCmd == "" {
		return Resolved{}, errors.New("DIOPTRA_LAYOUT_PROVIDER=cli requiere DIOPTRA_LAYOUT_CLI (ej. \"claude -p\")")
	}
	argv, err := SplitCommandLine(cliCmd)
	if err != nil {
		return Resolved{}, fmt.Errorf("DIOPTRA_LAYOUT_CLI inválido: %w", err)
	}
	if len(argv) == 0 {
		return Resolved{}, errors.New("DIOPTRA_LAYOUT_CLI vacío")
	}
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return Resolved{}, fmt.Errorf("DIOPTRA_LAYOUT_CLI: no se encontró %q en PATH: %w", argv[0], err)
	}
	source := "auto:cli"
	if explicit {
		source = "explicit"
	}
	resolvedArgv := append([]string{path}, argv[1:]...)
	return Resolved{
		Provider:       ProviderCLI,
		ProviderSource: source,
		Endpoint:       "exec: " + cliCmd,
		Model:          strings.TrimSuffix(filepath.Base(argv[0]), filepath.Ext(argv[0])),
		Command:        resolvedArgv,
	}, nil
}

// SplitCommandLine divide una línea de comando respetando comillas simples y dobles.
// La barra invertida es literal (rutas de Windows).
func SplitCommandLine(s string) ([]string, error) {
	var (
		args    []string
		cur     strings.Builder
		quote   rune
		inToken bool
	)
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
			inToken = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inToken {
				args = append(args, cur.String())
				cur.Reset()
				inToken = false
			}
		default:
			cur.WriteRune(r)
			inToken = true
		}
	}
	if quote != 0 {
		return nil, errors.New("comillas sin cerrar")
	}
	if inToken {
		args = append(args, cur.String())
	}
	return args, nil
}

// capBuffer acumula hasta max bytes y descarta el excedente sin bloquear al proceso hijo.
type capBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (c *capBuffer) Write(p []byte) (int, error) {
	if remaining := c.max - c.buf.Len(); remaining > 0 {
		if len(p) > remaining {
			c.buf.Write(p[:remaining])
			c.truncated = true
		} else {
			c.buf.Write(p)
		}
	} else if len(p) > 0 {
		c.truncated = true
	}
	return len(p), nil
}

// callCLI envía el prompt completo por stdin a la CLI ya autenticada del usuario
// (Claude Code, Antigravity, Codex…) y lee la respuesta de stdout.
func callCLI(ctx context.Context, r Resolved, system, user string) (providerReply, error) {
	if len(r.Command) == 0 {
		return providerReply{}, &ProviderError{Provider: ProviderCLI, Message: "comando vacío"}
	}
	prompt := system + "\n\n=== DOCUMENTO A MAQUETAR ===\n\n" + user

	cmd := exec.CommandContext(ctx, r.Command[0], r.Command[1:]...)
	cmd.Stdin = strings.NewReader(prompt)
	stdout := &capBuffer{max: maxCLIStdout}
	stderr := &capBuffer{max: maxCLIStderr}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = 2 * time.Second

	runErr := cmd.Run()
	if ctx.Err() != nil {
		return providerReply{}, &ProviderError{Provider: ProviderCLI, Message: fmt.Sprintf("%s interrumpido: %v", r.Model, ctx.Err())}
	}
	if runErr != nil {
		return providerReply{}, &ProviderError{Provider: ProviderCLI, Message: fmt.Sprintf("%s terminó con error (%v): %s", r.Model, runErr, tailSnippet(stderr.buf.String()))}
	}
	if stdout.truncated {
		return providerReply{}, fmt.Errorf("%s: %w (stdout > %d bytes)", ProviderCLI, ErrTruncated, maxCLIStdout)
	}
	out := stdout.buf.String()
	if strings.TrimSpace(out) == "" {
		return providerReply{}, &ProviderError{Provider: ProviderCLI, Message: fmt.Sprintf("%s no produjo salida en stdout: %s", r.Model, tailSnippet(stderr.buf.String()))}
	}
	return providerReply{text: out}, nil
}

func tailSnippet(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxErrorSnippet {
		s = "…" + s[len(s)-maxErrorSnippet:]
	}
	return s
}
