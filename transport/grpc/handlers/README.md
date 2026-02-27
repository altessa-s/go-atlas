# handlers

gRPC service handlers for go-atlas. Each handler implements the `server.Handler` interface and registers its proto-generated service with
a `grpc.Server`.

## Subpackages

| Package                          | Description                                                            |
|----------------------------------|------------------------------------------------------------------------|
| [health](./health)               | gRPC health checking protocol (grpc_health_v1) backed by Coordinator  |
| [scheduler](./scheduler)         | SchedulerService handler for task lifecycle and history management     |
