module lvmsync_go

go 1.24.3

require (
    go.uber.org/zap v0.0.0
    github.com/dustin/go-humanize v0.0.0
    github.com/spf13/pflag v0.0.0
    github.com/spf13/viper v0.0.0
    github.com/bits-and-blooms/bloom/v3 v3.0.0
    github.com/juju/ratelimit v0.0.0
    github.com/klauspost/compress/zstd v0.0.0
    github.com/pierrec/lz4/v4 v4.0.0
    golang.org/x/sys/cpu v0.0.0
)

replace go.uber.org/zap => ./stubs/go.uber.org/zap
replace github.com/dustin/go-humanize => ./stubs/github.com/dustin/go-humanize
replace github.com/spf13/pflag => ./stubs/github.com/spf13/pflag
replace github.com/spf13/viper => ./stubs/github.com/spf13/viper
replace github.com/bits-and-blooms/bloom/v3 => ./stubs/github.com/bits-and-blooms/bloom/v3
replace github.com/juju/ratelimit => ./stubs/github.com/juju/ratelimit
replace github.com/klauspost/compress/zstd => ./stubs/github.com/klauspost/compress/zstd
replace github.com/pierrec/lz4/v4 => ./stubs/github.com/pierrec/lz4/v4
replace golang.org/x/sys/cpu => ./stubs/golang.org/x/sys/cpu
