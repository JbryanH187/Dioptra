package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const sseHistoryLimit = 64

type sseEvent struct {
	ID   uint64
	Name string
	Data []byte
}

// SSEHub coordina las suscripciones en vivo de navegadores conectados a GET /api/events.
type SSEHub struct {
	mu sync.RWMutex
	// Por cliente: canal de eventos -> canal "kick" que se cierra para expulsarlo.
	clients map[chan sseEvent]chan struct{}
	nextID  uint64
	history []sseEvent
}

// NewSSEHub crea un hub pub/sub seguro para concurrencia.
func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[chan sseEvent]chan struct{}),
	}
}

// Broadcast emite un evento con nombre a todos los clientes SSE activos sin bloquear
// y conserva un historial acotado para reenviar tramas tras reconexiones (Last-Event-ID).
func (h *SSEHub) Broadcast(eventName string, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}

	h.mu.Lock()
	h.nextID++
	msg := sseEvent{ID: h.nextID, Name: eventName, Data: raw}
	h.history = append(h.history, msg)
	if len(h.history) > sseHistoryLimit {
		h.history = append([]sseEvent(nil), h.history[len(h.history)-sseHistoryLimit:]...)
	}
	// Si el buffer de un cliente lento está lleno NO se descarta el evento en
	// silencio: se expulsa al cliente; EventSource reconecta con Last-Event-ID y
	// recibe lo pendiente desde el historial.
	for ch, kick := range h.clients {
		select {
		case ch <- msg:
		default:
			delete(h.clients, ch)
			close(kick)
		}
	}
	h.mu.Unlock()
}

// ServeHTTP atiende conexiones GET /api/events con formato Server-Sent Events estándar.
func (h *SSEHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming SSE no soportado", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	var lastID uint64
	if rawLast := strings.TrimSpace(r.Header.Get("Last-Event-ID")); rawLast != "" {
		if parsed, err := strconv.ParseUint(rawLast, 10, 64); err == nil {
			lastID = parsed
		}
	}

	ch := make(chan sseEvent, 32)
	kick := make(chan struct{})
	var replay []sseEvent

	h.mu.Lock()
	h.clients[ch] = kick
	if lastID > 0 {
		for _, ev := range h.history {
			if ev.ID > lastID {
				replay = append(replay, ev)
			}
		}
	}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
	}()

	_, _ = fmt.Fprint(w, ": connected\n\nretry: 1500\n\n")
	for _, ev := range replay {
		_, _ = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.ID, ev.Name, ev.Data)
	}
	flusher.Flush()

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-kick:
			return
		case <-ticker.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev := <-ch:
			_, _ = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.ID, ev.Name, ev.Data)
			flusher.Flush()
		}
	}
}
