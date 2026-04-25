# storages

Scheduler storage implementations. Each subpackage implements the `scheduler.Storage` interface for persisting task state, execution
history, and pagination queries. Choose the backend that matches your deployment requirements.

## Subpackages

| Package                | Description                                                                              |
|------------------------|------------------------------------------------------------------------------------------|
| [memory](./memory)     | In-memory storage for testing and single-instance deployments                            |
| [mongodb](./mongodb)   | MongoDB storage with filter pushdown and paginated queries                               |
| [redis](./redis)       | Redis storage with sorted sets for scheduling and paginated queries                      |
