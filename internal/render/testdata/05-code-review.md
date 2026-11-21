# Revisión de código: PR #482

## Veredicto

**Aprobado con cambios.** La lógica es correcta, pero hay un problema de concurrencia.

## Cambio sugerido en el caché

```diff
--- a/internal/cache/lru.go
+++ b/internal/cache/lru.go
@@ -40,7 +40,9 @@ func (c *LRU) Get(k string) (V, bool) {
-	e, ok := c.items[k]
+	c.mu.Lock()
+	defer c.mu.Unlock()
+	e, ok := c.items[k]
 	if !ok {
```

## Comentarios por archivo

- `internal/cache/lru.go`: el mapa se lee sin candado. Ejemplo de la carrera:

  ```go
  go c.Get("a")
  go c.Set("a", 1)
  ```

- `internal/cache/lru_test.go`: falta un test con `-race`.

<details>
<summary>Salida completa de go vet</summary>

internal/cache/lru.go:41: possible data race on c.items

</details>
