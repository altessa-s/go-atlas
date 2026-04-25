# compression

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache/compression"
```

Package `compression` provides high-performance gzip compression for cache operations. Features 4-tier buffer pooling (small/medium/large via
`sync.Pool`), zero-copy for small payloads, streaming for large payloads, and zip bomb protection with configurable compression ratio limits.

## Key types

| Type / Interface   | Description                                                               |
|--------------------|---------------------------------------------------------------------------|
| `Compressor`       | Interface: Compress, Decompress, ShouldCompress                           |
| `GzipCompressor`   | Production implementation with size-based decisions and tiered pooling     |
| `Preset`           | Predefined compression configuration (None, Fast, Balanced, Best)         |
| `Metadata`         | Compressed data metadata: IsCompressed, OriginalSize                      |

## Presets

| Preset          | Level | Description                                                    |
|-----------------|-------|----------------------------------------------------------------|
| `PresetNone`    | --    | Compression disabled entirely                                  |
| `PresetFast`    | 1     | Fastest speed, moderate space savings (CPU-constrained)        |
| `PresetBalanced`| 6     | Optimal balance of speed and compression ratio (recommended)   |
| `PresetBest`    | 9     | Maximum compression ratio (bandwidth-constrained)              |

## Defaults

| Constant         | Value | Description                                  |
|------------------|-------|----------------------------------------------|
| `DefaultMinSize` | 1 KB  | Minimum payload size to trigger compression  |
| `DefaultMaxSize` | 0     | Maximum size (0 = no limit)                  |
| `DefaultLevel`   | 6     | Default gzip compression level               |
