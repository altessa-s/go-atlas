# Plugin Signature Verification Migration Guide

## Breaking Change: Signature Verification Now Required by Default

Starting with this version, plugin signature verification is **enabled by default** with mode `SignatureRequire`. This is a security enhancement to prevent loading of unsigned or tampered plugins.

### What Changed

- **Before**: `NewManager()` created a manager with signature verification disabled
- **Now**: `NewManager()` creates a manager with `SignatureRequire` mode, which rejects unsigned plugins

### Impact

If you are currently loading unsigned plugins, your application will fail to load them with an error:
```
plugin signature configuration error: plugin "example.so": signature required but no public key configured
```

### Migration Options

Choose one of the following approaches based on your security requirements:

#### Option 1: Sign Your Plugins (Recommended for Production)

1. Generate a key pair for signing:
```go
// Ed25519 (recommended)
openssl genpkey -algorithm ed25519 -out private.pem
openssl pkey -in private.pem -pubout -out public.pem

// Or ECDSA P-256
openssl ecparam -name prime256v1 -genkey -out private.pem
openssl ec -in private.pem -pubout -out public.pem

// Or RSA-PSS
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out private.pem
openssl rsa -in private.pem -pubout -out public.pem
```

2. Sign each plugin:
```bash
# For Ed25519
openssl pkeyutl -sign -inkey private.pem -in plugin.so -out plugin.so.sig

# For ECDSA
openssl dgst -sha256 -sign private.pem -binary plugin.so > plugin.so.sig

# For RSA-PSS
openssl dgst -sha256 -sign private.pem -sigopt rsa_padding_mode:pss plugin.so > plugin.so.sig
```

3. Configure the manager with your public key:
```go
mgr := plugins.NewManager(
    plugins.WithSignature(plugins.SignatureOptions{
        Mode:      plugins.SignatureRequire, // This is now the default
        PublicKey: publicKeyBytes,           // Your public key PEM bytes
    }),
    plugins.WithDir("./plugins"),
)
```

#### Option 2: Explicitly Disable Signature Verification (Development Only)

**SECURITY WARNING**: This disables signature verification completely, allowing any plugin to be loaded. Use only in development or controlled environments.

```go
mgr := plugins.NewManager(
    plugins.WithSignatureDisabled(),
    plugins.WithDir("./plugins"),
)
```

#### Option 3: Use Warning Mode (Transitional)

Log warnings for unsigned plugins but still load them:

```go
mgr := plugins.NewManager(
    plugins.WithSignature(plugins.SignatureOptions{
        Mode: plugins.SignatureWarn,
    }),
    plugins.WithDir("./plugins"),
)
```

### For Test Code

Tests that don't need signature verification can use the convenience function:

```go
func TestMyPlugin(t *testing.T) {
    mgr := plugins.NewManager(
        plugins.WithSignatureDisabled(),
        plugins.WithDir(t.TempDir()),
    )
    // ... test code
}
```

### Security Considerations

- **Production**: Always use signed plugins with `SignatureRequire` or `SignatureEnforce`
- **Development**: You may use `SignatureDisabled` or `SignatureWarn` for convenience
- **CI/CD**: Consider signing test plugins or using `WithSignatureDisabled()` in test environments
- **Key Management**: Protect your private signing key; distribute only the public key with your application

### Configuration via YAML

If using the factory package with configuration files:

```yaml
plugins:
  dir: "./plugins"
  signature:
    mode: "require"      # This is now the default
    publicKeyFile: "/path/to/public.pem"
```

To disable (not recommended for production):
```yaml
plugins:
  signature:
    mode: "disabled"
```

### Verification

After migration, verify your setup:

1. Unsigned plugins should be rejected (unless explicitly disabled)
2. Signed plugins with valid signatures should load successfully
3. Tampered or incorrectly signed plugins should be quarantined

### Need Help?

- See [docs/plugins.md](../docs/plugins.md) for complete signature verification documentation
- Review [plugins/signature_test.go](signature_test.go) for signing examples
- Check [plugins/factory/](factory/) for configuration examples