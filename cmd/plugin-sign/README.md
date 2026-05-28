# plugin-sign

Command-line tool for signing Go plugins with cryptographic signatures.

## Installation

```bash
go install github.com/altessa-s/go-atlas/cmd/plugin-sign@latest
```

## Usage

### Generate Key Pairs

First, generate a key pair for signing:

**Ed25519 (Recommended)**:
```bash
openssl genpkey -algorithm ed25519 -out private.pem
openssl pkey -in private.pem -pubout -out public.pem
```

**ECDSA P-256**:
```bash
openssl ecparam -name prime256v1 -genkey -out private.pem
openssl ec -in private.pem -pubout -out public.pem
```

**RSA-PSS**:
```bash
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out private.pem
openssl rsa -in private.pem -pubout -out public.pem
```

### Sign Plugins

Sign specific plugin files:
```bash
plugin-sign -key private.pem plugin1.so plugin2.so
```

Sign all `.so` files in a directory:
```bash
plugin-sign -key private.pem -dir ./plugins
```

Verbose output:
```bash
plugin-sign -key private.pem -dir ./plugins -v
```

### Verify Signatures

Verify plugin signatures:
```bash
plugin-sign -verify -pubkey public.pem plugin.so
```

Verify all plugins in a directory:
```bash
plugin-sign -verify -pubkey public.pem -dir ./plugins
```

## How It Works

1. **Signing**: Reads the plugin file, computes SHA-256 hash, signs the hash with the private key, and writes raw signature bytes to a `.sig`
   file.

2. **Verification**: Reads both the plugin and its `.sig` file, computes the plugin's SHA-256 hash, and verifies the signature matches using the
   public key.

3. **File Convention**: For each `plugin.so`, the signature is stored in `plugin.so.sig` in the same directory.

## Integration with go-atlas

The signed plugins work with the go-atlas plugin manager:

```go
import (
    "github.com/altessa-s/go-atlas/plugins"
)

// Configure manager with public key
mgr := plugins.NewManager(
    plugins.WithSignature(plugins.SignatureOptions{
        Mode:      plugins.SignatureRequire,
        PublicKey: publicKeyPEM,
    }),
    plugins.WithDir("./plugins"),
)

// Load signed plugins
if err := mgr.Load(ctx); err != nil {
    // Unsigned or tampered plugins will be rejected
}
```

## CI/CD Integration

### GitHub Actions

```yaml
- name: Sign plugins
  run: |
    echo "${{ secrets.PLUGIN_SIGN_KEY }}" > private.pem
    plugin-sign -key private.pem -dir ./plugins
    rm private.pem
```

### Docker

```dockerfile
# Sign plugins during build
RUN plugin-sign -key /keys/private.pem -dir /app/plugins
```

## Security Considerations

1. **Protect Private Keys**: Never commit private keys to version control. Use secrets management.

2. **Key Rotation**: Periodically rotate signing keys. The plugin manager can be configured with multiple public keys during transition.

3. **Algorithm Choice**: Ed25519 is recommended for its security, performance, and small key/signature sizes.

4. **Verification**: Always verify signatures in production. Use `SignatureRequire` or `SignatureEnforce` modes.

## Exit Codes

- `0`: Success
- `1`: Error (invalid arguments, missing files, signing/verification failure)

## Examples

### Full Workflow

```bash
# 1. Generate keys
openssl genpkey -algorithm ed25519 -out private.pem
openssl pkey -in private.pem -pubout -out public.pem

# 2. Build plugins
go build -buildmode=plugin -o auth.so ./plugins/auth
go build -buildmode=plugin -o cache.so ./plugins/cache

# 3. Sign plugins
plugin-sign -key private.pem auth.so cache.so

# 4. Verify signatures
plugin-sign -verify -pubkey public.pem auth.so cache.so

# 5. Deploy
# - Copy plugins and .sig files to production
# - Configure app with public.pem
```

### Batch Operations

Sign all plugins after build:
```bash
find ./plugins -name "*.so" -exec plugin-sign -key private.pem {} \;
```

Verify all before deployment:
```bash
plugin-sign -verify -pubkey public.pem -dir ./plugins || exit 1
```