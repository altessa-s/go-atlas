# handlers

gRPC service handlers for go-atlas. Each handler implements the `server.Handler` interface and registers its proto-generated service with a
`grpc.Server`.

## Subpackages

| Package                          | Description                                                                |
|----------------------------------|----------------------------------------------------------------------------|
| [health](./health)               | gRPC health checking protocol (grpc_health_v1) backed by Coordinator       |
| [scheduler](./scheduler)         | SchedulerService over a go-horai scheduler: CEL filters, page tokens       |
| [serviceinfo](./serviceinfo)     | ServiceInfoService reporting build, version, leader status, and uptime     |
