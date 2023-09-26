package main

import (
	"context"
	"flag"
	"os"

	"github.com/dehydr8/firecracker-containerd-agent-client/command"
	"github.com/google/subcommands"
)

func main() {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(subcommands.FlagsCommand(), "")
	subcommands.Register(subcommands.CommandsCommand(), "")
	subcommands.Register(&command.CallCmd{}, "")
	subcommands.Register(&command.ExecCmd{}, "")
	subcommands.Register(&command.CreateCmd{}, "")

	flag.Parse()
	ctx := context.Background()
	os.Exit(int(subcommands.Execute(ctx)))
}
