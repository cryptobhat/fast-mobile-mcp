module github.com/fast-mobile-mcp/worker-ios

go 1.22

require (
	github.com/fast-mobile-mcp/proto/gen/go v0.0.0
	github.com/fast-mobile-mcp/shared v0.0.0
	github.com/google/uuid v1.6.0
	google.golang.org/grpc v1.67.1
)

replace github.com/fast-mobile-mcp/shared => ../shared/go
replace github.com/fast-mobile-mcp/proto/gen/go => ../proto/gen/go
