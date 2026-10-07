# storagetest

```go
import "github.com/altessa-s/go-atlas/data/audit/storages/storagetest"
```

Conformance suite of `audit.Storage` implementations: every backend under `data/audit/storages` runs `Run` to prove it implements the ordering,
paging and `Count` contract that `audit.FetchPage` relies on. A custom storage should run it too.

## Contracts

| Subtest               | Verifies                                                                                               |
|-----------------------|--------------------------------------------------------------------------------------------------------|
| `Order`               | Total `(timestamp millisecond, ID)` order in both directions, at every page size                       |
| `PageBoundaries`      | Exact page boundaries: a full last page carries no next token                                          |
| `InsertsDuringPaging` | Events inserted ahead of the position appear on later pages; events behind it do not                   |
| `Count`               | `Count` matches the filter and ignores the page position and size                                      |
| `TimeRange`           | `StartTime`/`EndTime` bound whole milliseconds                                                         |
| `Replay`              | A replayed event is accepted without duplicating results, or rejected as recognized by `rejectsReplay` |
| `TokenBindings`       | A page token is bound to the filter, the sort order and the subject, and rejected when tampered with   |

## Usage

```go
func TestConformance(t *testing.T) {
	storagetest.Run(t, func(*testing.T) audit.Storage { return memory.New() }, nil)
}
```
