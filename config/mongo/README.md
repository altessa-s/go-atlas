# mongoconfig

```go
import mongoconfig "github.com/altessa-s/go-atlas/config/mongo"
```

Package `mongoconfig` defines the MongoDB connection, credential and client-side field level encryption schemas. Schemas are populated by
[`config/loader`](../loader) and consumed by the component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type                     | Description                                                                                                    |
|--------------------------|----------------------------------------------------------------------------------------------------------------|
| `Config`                | Represents the configuration for MongoDB database connections.                                                 |
| `CompressionType`   | Represents the compression algorithm type for MongoDB connections.                                             |
| `CompressionTypes`  | Represents a slice of compression types.                                                                       |
| `AuthMechanismType` | Represents the authentication mechanism type for MongoDB.                                                      |
| `PLAINCredentials`  | Represents the credentials for the PLAIN authentication mechanism.                                             |
| `SCRAMCredentials`  | Represents the credentials for the SCRAM authentication mechanism.                                             |
| `Credentials`     | Represents the credentials for the MongoDB connection.                                                         |
| `KMSProvider`       | Represents the type of Key Management Service provider for MongoDB client-side field level encryption (CSFLE). |
| `KMS`               | Represents the Key Management Service configuration for MongoDB encryption.                                    |
| `KMSLocal`          | Represents the local Key Management Service provider.                                                          |
| `KMSAmazon`         | Represents the AWS Key Management Service provider configuration.                                              |
| `KMSAzure`          | Represents the Azure Key Vault provider configuration.                                                         |
| `KMSGoogle`         | Represents the Google Cloud KMS provider configuration.                                                        |
| `EncryptionType`    | Represents the type of client-side field level encryption.                                                     |
| `Encryption`        | Represents the client-side field level encryption configuration for MongoDB.                                   |

See the [config index](../README.md) for the other schema packages.
