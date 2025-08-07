module lvmsync_go

go 1.24.5

require (
	github.com/bits-and-blooms/bloom/v3 v3.7.0
	github.com/klauspost/compress v1.18.0
	github.com/klauspost/cpuid/v2 v2.2.7
	github.com/nak3/go-lvm v0.0.0
	github.com/pierrec/lz4/v4 v4.1.22
	github.com/spf13/pflag v1.0.7
	github.com/spf13/viper v1.20.1
	github.com/zeebo/blake3 v0.2.4
	go.uber.org/multierr v1.10.0
	go.uber.org/zap v1.27.0
	golang.org/x/crypto v0.40.0
	golang.org/x/sys v0.34.0
	golang.org/x/time v0.12.0
)

replace github.com/nak3/go-lvm => ./stubs/github.com/nak3/go-lvm

require (
	github.com/bits-and-blooms/bitset v1.10.0 // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.2.1 // indirect
	github.com/pelletier/go-toml/v2 v2.2.3 // indirect
	github.com/sagikazarmark/locafero v0.7.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.12.0 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	golang.org/x/text v0.27.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
