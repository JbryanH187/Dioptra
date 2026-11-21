# Auditoría de seguridad: API de pagos

Revisé **12 archivos** del servicio `payments-api` y encontré *tres* problemas críticos. El detalle de cada uno está abajo, con su ubicación exacta.

## Resumen ejecutivo

La autenticación está bien resuelta, pero el manejo de reintentos filtra goroutines y el endpoint de reembolsos no valida el monto.

> [!WARNING] Riesgo principal
> Un atacante puede pedir reembolsos mayores al cargo original (internal/refunds/handler.go:57).

## Hallazgos

### Críticos

| Severidad | Hallazgo | Ubicación |
| --- | --- | --- |
| error | Reembolso sin validar monto | internal/refunds/handler.go:57 |
| error | Fuga de goroutines en reintentos | internal/payments/retry.go:44 |
| warning | Logs con número de tarjeta parcial | internal/log/mask.go:12 |

### Menores

- El timeout del cliente HTTP es de 60 s; conviene bajarlo a 10 s.
- Falta `context` en tres funciones públicas:
  - `Charge` en internal/payments/charge.go:20
  - `Refund` en internal/refunds/refund.go:31
  - `Void` en internal/payments/void.go:9

## Recomendaciones

1. Validar `amount <= original.amount` antes de crear el reembolso.
2. Cerrar el canal de respuesta en el `select` de timeout.
3. Enmascarar la tarjeta completa con `mask.PAN()`.

## Próximos pasos

- [ ] Parchear validación de monto @backend [error]
- [ ] Agregar test de regresión para la fuga @backend
- [x] Confirmar alcance con el equipo de riesgo @seguridad
