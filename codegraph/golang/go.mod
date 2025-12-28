module github.com/humanbeeng/lepo/prototypes/codegraph

go 1.24.0

toolchain go1.24.5

require (
	basegraph.app/relay v0.0.0
	github.com/joho/godotenv v1.5.1
	golang.org/x/tools v0.38.0
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/arangodb/go-driver/v2 v2.1.6 // indirect
	github.com/arangodb/go-velocypack v0.0.0-20200318135517-5af53c29c67e // indirect
	github.com/dchest/siphash v1.2.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/kkdai/maglev v0.2.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/oapi-codegen/runtime v1.1.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	github.com/sony/gobreaker v1.0.0 // indirect
	github.com/typesense/typesense-go/v4 v4.0.0-alpha2 // indirect
	golang.org/x/mod v0.29.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sync v0.18.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.31.0 // indirect
)

replace basegraph.app/relay => ../../relay
