# mongo

MongoDB infrastructure for the Atlas framework. Provides configuration-based creation of MongoDB driver clients and higher-level
`mongo.Mongo` wrappers with authentication, TLS, connection pooling, compression, and client-side field level encryption (CSFLE).

## Subpackages

| Package              | Description                                                                       |
|----------------------|-----------------------------------------------------------------------------------|
| [factory](./factory) | Configuration-based `mongo.Mongo` and `mongo/options.ClientOptions` creation      |
