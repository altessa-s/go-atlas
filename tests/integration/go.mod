// Integration suite for github.com/altessa-s/go-atlas.
//
// A separate module so the SQL drivers it needs to exercise the SQL
// storages — pgx, go-sql-driver/mysql — stay out of the toolkit's own
// dependency graph. Consumers of go-atlas never resolve them. clickhouse-go
// is a root dependency of the ClickHouse audit storage.
module github.com/altessa-s/go-atlas/tests/integration

go 1.27.0

replace github.com/altessa-s/go-atlas => ../..

require (
	github.com/altessa-s/go-aip v0.1.8
	github.com/altessa-s/go-aip/backend/mongo v0.1.8
	github.com/altessa-s/go-atlas v0.0.0
	github.com/altessa-s/go-atlas-core v0.3.5
	github.com/go-sql-driver/mysql v1.10.0
	github.com/jackc/pgx/v5 v5.10.0
	github.com/nats-io/nats.go v1.55.0
	github.com/redis/go-redis/v9 v9.23.0
	github.com/shopspring/decimal v1.6.0
	github.com/stretchr/testify v1.12.1
	github.com/xakep666/mongo-migrate v0.4.1
	go.mongodb.org/mongo-driver/v2 v2.10.0
)

require (
	cel.dev/cel-go v0.33.2 // indirect
	cel.dev/expr v0.25.3 // indirect
	cloud.google.com/go v0.123.0 // indirect
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/adrg/postcode v0.1.0 // indirect
	github.com/altessa-s/go-convert v0.1.0 // indirect
	github.com/altessa-s/go-horai v0.1.2 // indirect
	github.com/altessa-s/go-kryptos v0.1.4 // indirect
	github.com/altessa-s/go-nomos v0.2.3 // indirect
	github.com/altessa-s/go-slogx v0.1.4 // indirect
	github.com/altessa-s/ozzo-rules v1.17.0 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/bits-and-blooms/bitset v1.24.2 // indirect
	github.com/bits-and-blooms/bloom/v3 v3.7.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-ozzo/ozzo-validation/v4 v4.5.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/compress v1.20.4 // indirect
	github.com/nats-io/nkeys v0.4.17 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/nyaruka/phonenumbers v1.8.1 // indirect
	github.com/oklog/ulid/v2 v2.1.2 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/xdg-go/scram v1.4.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	go.uber.org/atomic v1.12.0 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/exp v0.0.0-20260908205506-85c1c2202aba // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20261110000943-09e21b237f2c // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261110000943-09e21b237f2c // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)
