# Optimización del endpoint /search

Bajamos el p95 de **820 ms a 140 ms** (-83%).

## Antes y después

| Métrica | Antes | Después |
| --- | --- | --- |
| p50 (ms) | 310 | 60 |
| p95 (ms) | 820 | 140 |
| Allocs/op | 1,940 | 212 |

## Qué cambió

1. Índice GIN sobre `products.search_vector`.
2. Se eliminó un `N+1` en `internal/search/repo.go:118`.
3. Pool de `bytes.Buffer` con `sync.Pool`.

```go
var bufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}
```

## Pendiente

Medir con tráfico real durante una semana antes de cerrar el ticket.
