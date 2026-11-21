# Postmortem: caída del checkout (2026-11-12)

```kpi
Duración | 47 | min | | error
Pedidos perdidos | 1,240 | | |
Clientes afectados | 8.3 | % | | warning
MTTR objetivo | 30 | min | |
```

## Línea de tiempo

| Hora | Evento |
| --- | --- |
| 14:02 | Deploy de `checkout v4.18` |
| 14:05 | Primeras alertas 5xx |
| 14:31 | Rollback iniciado |
| 14:49 | Servicio estable |

## Causa raíz

El nuevo pool de conexiones tenía `max_conns = 5` en lugar de `50` por un typo en `deploy/values-prod.yaml:88`.

> [!NOTE] Por qué no lo detectaron las pruebas
> Staging usa `values-staging.yaml`, que no tenía el typo.

## Acciones

- [ ] Validar valores de prod en CI @sre [error]
- [ ] Alerta de saturación del pool @observabilidad
