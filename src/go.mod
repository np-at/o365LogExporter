module o365logexporter

go 1.17

replace k8s.io/client-go => k8s.io/client-go v12.0.0+incompatible // indirect

require (
	github.com/cornelk/hashmap v1.0.1
	github.com/golang/snappy v0.0.4
	github.com/grafana/loki v1.6.2-0.20211108122114-f61a4d2612d8
	github.com/urfave/cli/v2 v2.3.0
)

require (
	github.com/BurntSushi/toml v0.3.1 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/dchest/siphash v1.1.0 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/russross/blackfriday/v2 v2.0.1 // indirect
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
