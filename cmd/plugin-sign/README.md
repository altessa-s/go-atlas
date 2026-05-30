# plugin-sign

Command-line tool for generating signing keys and signing/verifying Go plugins with cryptographic signatures.

## Installation

```bash
go install github.com/altessa-s/go-atlas/cmd/plugin-sign@latest
```

## CLI shape

`plugin-sign` is a verb-style CLI:

```
plugin-sign keygen [flags]
plugin-sign sign   [flags] [plugin.so ...]
plugin-sign verify [flags] [plugin.so ...]
plugin-sign -h
```

Run `plugin-sign <command> -h` to see flags for a specific command.

## Usage

### Generate Key Pairs (built-in)

Generate an Ed25519 key pair (recommended):

```bash
plugin-sign keygen -alg ed25519 -priv-out private.pem -pub-out public.pem
```

ECDSA P-256:

```bash
plugin-sign keygen -alg ecdsa -priv-out private.pem -pub-out public.pem
```

RSA-PSS (default 3072 bits, range 2048–4096):

```bash
plugin-sign keygen -alg rsa -rsa-bits 3072 -priv-out private.pem -pub-out public.pem
```

By default `keygen` refuses to overwrite existing files — pass `-force` to replace them. The private key is written with mode `0600`; the public
key with `0644`. Output format: PKCS#8 PEM for the private half, PKIX PEM for the public half — directly consumable by `sign`/`verify`.

#### Alternative: openssl

If you prefer `openssl` over the built-in generator:

```bash
# Ed25519
openssl genpkey -algorithm ed25519 -out private.pem
openssl pkey -in private.pem -pubout -out public.pem

# ECDSA P-256
openssl ecparam -name prime256v1 -genkey -out private.pem
openssl ec -in private.pem -pubout -out public.pem

# RSA-PSS
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 -out private.pem
openssl rsa -in private.pem -pubout -out public.pem
```

`plugin-sign sign` accepts PKCS#8, PKCS#1, SEC1 EC, and raw Ed25519-seed private keys, so all of the above work.

### Sign Plugins

Sign specific plugin files:

```bash
plugin-sign sign -key private.pem plugin1.so plugin2.so
```

Sign all `.so` files in a directory:

```bash
plugin-sign sign -key private.pem -dir ./plugins
```

Verbose output:

```bash
plugin-sign sign -key private.pem -dir ./plugins -v
```

### Verify Signatures

Verify a plugin signature:

```bash
plugin-sign verify -pubkey public.pem plugin.so
```

Verify all plugins in a directory:

```bash
plugin-sign verify -pubkey public.pem -dir ./plugins
```

## How It Works

1. **Keygen**: Produces a key pair. Private half is `x509.MarshalPKCS8PrivateKey` wrapped in a `PRIVATE KEY` PEM block; public half is
   `x509.MarshalPKIXPublicKey` wrapped in a `PUBLIC KEY` PEM block.

2. **Signing**: Reads the plugin file, computes SHA-256 hash, signs the hash with the private key, and writes raw signature bytes to a
   `.sig` file.

3. **Verification**: Reads both the plugin and its `.sig` file, computes the plugin's SHA-256 hash, and verifies the signature matches using
   the public key.

4. **File convention**: For each `plugin.so`, the signature is stored in `plugin.so.sig` in the same directory.

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
    plugin-sign sign -key private.pem -dir ./plugins
    rm private.pem
```

### Docker

```dockerfile
# Sign plugins during build
RUN plugin-sign sign -key /keys/private.pem -dir /app/plugins
```

## Security Considerations

1. **Protect private keys**: Never commit private keys to version control. `keygen` writes the private file with `0600` to start, but storage
   and transport are your responsibility — use secrets management.

2. **Key rotation**: Periodically rotate signing keys. The plugin manager can be configured with multiple public keys during transition.

3. **Algorithm choice**: Ed25519 is recommended for its security, performance, and small key/signature sizes.

4. **Verification**: Always verify signatures in production. Use `SignatureRequire` or `SignatureEnforce` modes.

## Exit Codes

| Code | Meaning |
|------|---------|
| `0`  | Success |
| `1`  | Runtime error (signing/verification failed, key load failed, I/O error) |
| `2`  | Usage error (unknown command, missing required flags, etc.) |

## Examples

### Full Workflow

```bash
# 1. Generate keys
plugin-sign keygen -alg ed25519 -priv-out private.pem -pub-out public.pem

# 2. Build plugins
go build -buildmode=plugin -o auth.so ./plugins/auth
go build -buildmode=plugin -o cache.so ./plugins/cache

# 3. Sign plugins
plugin-sign sign -key private.pem auth.so cache.so

# 4. Verify signatures
plugin-sign verify -pubkey public.pem auth.so cache.so

# 5. Deploy
# - Copy plugins and .sig files to production
# - Configure app with public.pem
```

### Batch Operations

Sign all plugins after build:

```bash
find ./plugins -name "*.so" -exec plugin-sign sign -key private.pem {} \;
```

Verify all before deployment:

```bash
plugin-sign verify -pubkey public.pem -dir ./plugins || exit 1
```

## Migrating from the previous CLI

The previous release used a flag-only interface with `-verify` as a mode toggle. The verb-style CLI is a breaking change — update scripts as
follows:

| Before                                              | After                                           |
|-----------------------------------------------------|-------------------------------------------------|
| `plugin-sign -key private.pem plugin.so`            | `plugin-sign sign -key private.pem plugin.so`   |
| `plugin-sign -key private.pem -dir ./plugins`       | `plugin-sign sign -key private.pem -dir ./plugins` |
| `plugin-sign -verify -pubkey public.pem plugin.so`  | `plugin-sign verify -pubkey public.pem plugin.so` |
| `openssl genpkey -algorithm ed25519 -out priv.pem` (+ `openssl pkey -pubout`) | `plugin-sign keygen -alg ed25519 -priv-out priv.pem -pub-out pub.pem` |
