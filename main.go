package main

import (
	"github.com/riba2534/feishu-cli/v2/cmd"
)

// Version information, set by ldflags during build
var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	cmd.SetVersionInfo(Version, BuildTime)
	cmd.Execute()
}
