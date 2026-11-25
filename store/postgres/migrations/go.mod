// PostgreSQL migration registry of github.com/altessa-s/go-atlas.
//
// A separate module because goose's go.mod requires the drivers and SDKs of
// every database goose supports (pgx, go-sql-driver/mysql, go-mssqldb,
// sqlite, ydb, ClickHouse, docker client, ...): in the root module they would
// enter the dependency graph of every consumer of the root PostgreSQL runtime. See docs/adr/0001-postgres-migrations-module.md.
module github.com/altessa-s/go-atlas/store/postgres/migrations

go 1.27.0

replace github.com/altessa-s/go-atlas => ../../..

require (
	github.com/altessa-s/go-atlas v0.0.0-20261124092112-3768e72efb7f
	github.com/jackc/pgx/v5 v5.12.0
	github.com/pressly/goose/v3 v3.30.1
	github.com/stretchr/testify v1.12.1
)

require (
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/altessa-s/go-atlas-core v0.3.5 // indirect
	github.com/altessa-s/go-horai v0.1.2 // indirect
	github.com/altessa-s/go-optgen v0.4.0 // indirect
	github.com/altessa-s/go-slogx v0.1.4 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mfridman/interpolate v0.2.0 // indirect
	github.com/sethvargo/go-retry v0.5.0 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.1.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

tool github.com/altessa-s/go-optgen/cmd/optgen
