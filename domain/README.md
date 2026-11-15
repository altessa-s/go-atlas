# domain

The `domain` directory holds domain value types. The packages depend only on the standard library, go-atlas-core and decimal
arithmetic; structural mapping lives in [mapping](../mapping), message passing in [messaging](../messaging).

## Subpackages

| Package                          | Description                                                                            |
|----------------------------------|----------------------------------------------------------------------------------------|
| `money`                          | Currency amounts with exact decimal arithmetic, rounding to minor units and allocation |
