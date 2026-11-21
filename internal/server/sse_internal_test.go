package server

import "testing"

// Un cliente con el buffer lleno se expulsa (para que reconecte con Last-Event-ID)
// en vez de perder eventos en silencio.
func TestSSEHub_SlowClientIsEvictedNotSilentlyDropped(t *testing.T) {
	h := NewSSEHub()
	ch := make(chan sseEvent, 32)
	kick := make(chan struct{})
	h.clients[ch] = kick

	for i := 0; i < 33; i++ {
		h.Broadcast("report_updated", map[string]any{"n": i})
	}

	select {
	case <-kick:
	default:
		t.Fatal("el cliente lento debía ser expulsado al llenarse su buffer")
	}
	if _, still := h.clients[ch]; still {
		t.Fatal("el cliente expulsado no debe seguir registrado")
	}
	if last := h.history[len(h.history)-1]; last.ID != 33 {
		t.Fatalf("el evento 33 debe quedar en el historial para el replay, último=%d", last.ID)
	}
	// Broadcasts posteriores no deben entrar en pánico (canal kick ya cerrado).
	h.Broadcast("report_updated", map[string]any{"n": 34})
}
