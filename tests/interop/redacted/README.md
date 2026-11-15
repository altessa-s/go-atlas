# tests/interop/redacted

Interoperability tests of [`go-atlas-core/types/redacted`](https://github.com/altessa-s/go-atlas-core/tree/develop/types/redacted) with third-party
encoders: `gopkg.in/yaml.v3` and the MongoDB BSON codec. They live outside `core` so the core module does not require those libraries, even for tests.
JSON and text encodings stay tested in the package itself.
