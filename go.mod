module github.com/panyam/devloop

go 1.24.0

require (
	connectrpc.com/connect v1.18.1
	github.com/bmatcuk/doublestar/v4 v4.8.1
	github.com/fatih/color v1.18.0
	github.com/felixge/httpsnoop v1.0.4
	github.com/fsnotify/fsnotify v1.9.0
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.1
	github.com/mark3labs/mcp-go v0.32.0
	github.com/panyam/gocurrent v0.0.2
	github.com/pelletier/go-toml/v2 v2.2.4
	github.com/redpanda-data/protoc-gen-go-mcp v0.0.0-20250614184940-a304d5967ba0
	github.com/spf13/cobra v1.9.1
	github.com/stretchr/testify v1.10.0
	google.golang.org/genproto/googleapis/api v0.0.0-20250603155806-513f23925822
	google.golang.org/grpc v1.73.0
	google.golang.org/protobuf v1.36.6
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/exp/typeparams v0.0.0-20231108232855-2478ac86f678 // indirect
	golang.org/x/mod v0.25.0 // indirect
	golang.org/x/net v0.40.0 // indirect
	golang.org/x/sync v0.15.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	golang.org/x/tools v0.33.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250603155806-513f23925822 // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.5.1 // indirect
	honnef.co/go/tools v0.6.1 // indirect
)

// This is needed till the custom tool names annotations PR (#16) is merged
replace github.com/redpanda-data/protoc-gen-go-mcp v0.0.0-20250614184940-a304d5967ba0 => ./locallinks/protoc-gen-go-mcp/
