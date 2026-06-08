# service

Application-level services for the Atlas framework. Provides reusable service components that sit between the transport layer and
the data layer, encapsulating business logic for common cross-cutting concerns like scheduling and ID generation.

## Subpackages

| Package                    | Description                                                                            |
|----------------------------|----------------------------------------------------------------------------------------|
| [dispatch](./dispatch)     | Async batching dispatcher with optional WAL durability for at-least-once delivery      |
| [id](./id)                 | Distributed unique ID generation                                                       |
| [scheduler](./scheduler)   | Background task scheduling with cron expressions, intervals, and lifecycle management  |
