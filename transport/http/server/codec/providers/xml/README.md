# xml

```go
import "github.com/altessa-s/go-atlas/transport/http/server/codec/providers/xml"
```

Package `xml` provides an XML `codec.Codec` backed by `encoding/xml`. The codec is safe for concurrent use and supports two MIME types:
`application/xml` (primary) and `text/xml` (alternate). Both are registered in the global `DefaultRegistry` at init time. An optional
XML declaration header (`<?xml version="1.0" encoding="UTF-8"?>`) can be prepended to encoded output.

## Options

| Option       | Default | Description                                                                              |
|--------------|---------|------------------------------------------------------------------------------------------|
| `WithIndent` | false   | Enable pretty-printed XML output with indentation                                        |
| `WithHeader` | false   | Prepend the standard XML declaration header to encoded output                            |
