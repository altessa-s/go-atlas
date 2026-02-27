# plugin

```go
import "github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
```

Package `plugin` defines the public extension API for the optgen code generator. Lives outside `/internal` so that external `.so`
plugins and third-party libraries can implement optgen extensions with stable interfaces.

## Plugin types

| Interface          | Description                                                                               |
|--------------------|-------------------------------------------------------------------------------------------|
| `FieldPlugin`      | Selects and generates the `WithXxx` function for a field (primary plugin type)            |
| `TransformPlugin`  | Applies a single `optval` modifier (e.g. `lower`, `trimspaces`) to an expression         |
| `ModifierPlugin`   | Unified pipeline modifier for guards, post-processing, and checks                        |
| `TypeDefaultPlugin`| Supplies a default-value expression for a field type when none is specified               |
| `CheckPlugin`      | Generates validation code for an `optcheck` key (e.g. `required`, `minlen`)              |

## Pipeline phases

| Phase          | Order | Description                                                                          |
|----------------|-------|--------------------------------------------------------------------------------------|
| Guard          | 1     | Pre-assignment checks (e.g. `notnil`)                                                |
| Transform      | 2     | Value transformations (e.g. `lower`, `trimspaces`, `dedup`)                          |
| PostProcess    | 3     | Post-assignment processing (e.g. `positive`, `nonempty`)                             |
| Check          | 4     | Validation (e.g. `required`, `minlen`, `maxlen`, `oneof`)                            |

## Registration

Plugins register themselves in `init()` via the global `Register` function which dispatches to the appropriate `Registry` method by
type-assertion. The global `Registry` is protected by `sync.RWMutex` and safe for concurrent reads during generation.

## Key types

| Type               | Description                                                                              |
|--------------------|------------------------------------------------------------------------------------------|
| `Registry`         | Thread-safe registry for all plugin types with priority-based lookup                     |
| `Meta`             | Plugin metadata: name, kind, priority, and applicable types                              |
| `GenerationContext`| Per-field context passed to plugins with struct info, imports, and helpers                |
| `GeneratedCode`    | Output of `FieldPlugin.Generate`: code, type definitions, and helper functions           |
