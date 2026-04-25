# http

HTTP transport subsystem for the Atlas framework. Provides a resilient HTTP client and a fully-featured HTTP server with middleware,
content negotiation, and pluggable routing.

## Packages

| Package              | Description                                                                            |
|----------------------|----------------------------------------------------------------------------------------|
| [client](./client)   | Resilient HTTP client with retries, circuit breaker, rate limiting, and SSRF protection |
| [server](./server)   | HTTP server with graceful shutdown, middleware support, and pluggable routers           |
