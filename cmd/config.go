package cmd

import (
	"github.com/loadimpact/k6/config"
	"github.com/spf13/pflag"
)

// Gets configuration from CLI flags.
func getConfig(flags *pflag.FlagSet) (config.Config, error) {
	opts, err := getOptions(flags)
	if err != nil {
		return config.Config{}, err
	}
	out, err := flags.GetStringArray("out")
	if err != nil {
		return config.Config{}, err
	}
	return config.Config{
		Options:       opts,
		Out:           out,
		Linger:        getNullBool(flags, "linger"),
		NoUsageReport: getNullBool(flags, "no-usage-report"),
		NoThresholds:  getNullBool(flags, "no-thresholds"),
		NoSummary:     getNullBool(flags, "no-summary"),
	}, nil
}
