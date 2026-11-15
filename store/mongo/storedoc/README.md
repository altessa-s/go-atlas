# storedoc

```go
import "github.com/altessa-s/go-atlas/store/mongo/storedoc"
```

Classifies BSON document decoding failures using a caller-owned error category. The package has no application-specific errors or storage state.

## API

| Function | Behavior |
|----------|----------|
| `Classify(err, category)` | Wraps BSON decoding failures with category while preserving the decoder error; unrelated failures pass through unchanged. |

## Usage

```go
var ErrCorruptRecord = errors.New("stored record is invalid")

err := collection.FindOne(ctx, filter).Decode(&document)
return storedoc.Classify(err, ErrCorruptRecord)
```

Call it at the read boundary, before adding operation context: after `Decode`, `cursor.All`, `cursor.Decode`, or a repository read helper.
It recognizes `*bson.DecodeError` and `bson.ValueDecoderError` anywhere in the error chain. Both the original failure and the supplied category
remain reachable through `errors.Is` / `errors.As`. An error already matching category is returned unchanged, so classification is idempotent.

Nil errors, a nil category, missing documents, and query or transport failures pass through unchanged. Application validation failures are the
caller's responsibility; this helper neither parses error messages nor labels every MongoDB error as corrupt data.
