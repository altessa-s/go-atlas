# redis

Redis infrastructure for the Atlas framework. Provides configuration-based creation of Redis clients with automatic mode detection
(standalone, sentinel, cluster), authentication, connection pooling, and health-check integration.

## Subpackages

| Package              | Description                                                                       |
|----------------------|-----------------------------------------------------------------------------------|
| [factory](./factory) | Configuration-based `redis.UniversalClient` creation with mode auto-detection     |
