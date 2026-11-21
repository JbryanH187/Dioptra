# Plan de migración a PostgreSQL 17

## Contexto

Hoy corremos PostgreSQL 13, que sale de soporte en noviembre. La migración afecta a 4 servicios.

## Pasos

1. Preparar el entorno
   1. Crear réplica lógica en la versión 17.
   2. Validar extensiones: `pg_trgm`, `pgcrypto`.
2. Migrar datos
   - Usar `pg_upgrade --link` en staging.
   - Medir tiempo de corte; objetivo < 5 min.
3. Cortar tráfico
   1. Poner servicios en modo solo lectura.
   2. Promover réplica.
   3. Reapuntar los DSN.

## Riesgos

> Si la réplica se atrasa más de 30 s, el corte se pospone. Esta es una regla del equipo de datos, no una sugerencia.

## Tareas

- [ ] Crear réplica en staging @datos
  - [ ] Validar extensiones @datos
- [ ] Ensayo de corte @sre [warning]
- [ ] Comunicar ventana de mantenimiento @pm
