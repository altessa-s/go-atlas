// Integration suite for github.com/altessa-s/go-atlas.
//
// A separate module so the SQL drivers it needs to exercise the filter
// translators — clickhouse-go, pgx, go-sql-driver/mysql — stay out of
// the toolkit's own dependency graph. Consumers of go-atlas never
// resolve them.
module github.com/altessa-s/go-atlas/tests/integration

go 1.25.0

replace github.com/altessa-s/go-atlas => ../..

require (
	github.com/ClickHouse/clickhouse-go/v2 v2.40.3
	github.com/altessa-s/go-atlas v0.0.0
	github.com/go-sql-driver/mysql v1.10.0
	github.com/jackc/pgx/v5 v5.10.0
	github.com/redis/go-redis/v9 v9.21.0
	github.com/stretchr/testify v1.11.1
	go.mongodb.org/mongo-driver/v2 v2.8.0
)

require (
	cel.dev/expr v0.25.2 // indirect
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/ClickHouse/ch-go v0.68.0 // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/go-faster/city v1.0.1 // indirect
	github.com/go-faster/errors v0.7.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/cel-go v0.30.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/klauspost/compress v1.19.1 // indirect
	github.com/meilisearch/meilisearch-go v0.36.3 // indirect
	github.com/paulmach/orb v0.11.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.6.1 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.2.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/exp v0.0.0-20250813145105-42675adae3e6 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260729162451-8efbd57d26e0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260729162451-8efbd57d26e0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
