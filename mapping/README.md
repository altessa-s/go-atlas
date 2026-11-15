# mapping

Structural mapping between representations: struct-tag driven field behavior and field change tracking. Struct-to-struct
conversion itself is the external [go-convert](https://github.com/altessa-s/go-convert) module.

## Packages

| Package                      | Description                                                                                       |
|------------------------------|---------------------------------------------------------------------------------------------------|
| [behavior](./behavior)       | Struct-tag behavior strip/clean core with pluggable output translators (`translators/mongo`) ([guide](../docs/mapping/behavior.md)) |
| [fieldtracker](./fieldtracker) | Detect changed fields between two struct instances                                              |
