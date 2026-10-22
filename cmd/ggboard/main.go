// Command ggboard is the GG Bond panel server and CLI.
//
// Plugins and extra subcommands are linked by blank imports in sibling files of this package
// (e.g. plugins.go importing payment gateways and subscription formats), never by editing this file.
package main

import (
	"os"

	"github.com/linusxiong/ggboard/internal/cli"
)

// version is set at build time: go build -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	os.Exit(cli.Execute(version))
}
