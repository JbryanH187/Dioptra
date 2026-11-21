# Arquitectura del motor de notificaciones

El sistema recibe eventos de dominio y los entrega por canal según las preferencias del usuario.

## Flujo principal

```mermaid
flowchart LR
    A[API Gateway] -->|evento| B(Router de eventos)
    B --> C{¿Canal?}
    C -->|email| D[Worker SMTP]
    C -->|push| E[Worker FCM]
    C -->|sms| F[Worker Twilio]
    subgraph Persistencia
      G[(PostgreSQL)]
    end
    B --> G
```

## Componentes

| Componente | Lenguaje | Réplicas | Latencia p95 (ms) |
| --- | --- | --- | --- |
| Router | Go | 3 | 12 |
| Worker SMTP | Go | 2 | 340 |
| Worker FCM | Node | 2 | 95 |

## Decisiones

La cola es **Kafka** porque necesitamos reprocesar eventos (ver docs/adr/0007-kafka.md).

---

Diagrama de despliegue:

![Despliegue en Kubernetes](docs/img/deploy.png)
