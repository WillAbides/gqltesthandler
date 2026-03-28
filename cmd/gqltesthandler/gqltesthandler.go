package main

import (
	"github.com/alecthomas/kong"
	"github.com/willabides/gqltesthandler/internal/handlergen"
)

var version = "unknown"

const description = `Generates mock GraphQL test handlers from a schema and predefined operations.`

type cmdRoot struct {
	Schema     string           `kong:"required,help='Path to GraphQL schema file'"`
	Operations string           `kong:"required,help='Path to GraphQL operations file'"`
	Out        string           `kong:"short='o',required,help='Directory to write the generated test handler to'"`
	Version    kong.VersionFlag `kong:"help=${VersionHelp}"`
}

var kongVars = kong.Vars{
	"version":     version,
	"VersionHelp": `Output the gqltesthandler version and exit.`,
}

func main() {
	var cli cmdRoot
	k := kong.Parse(&cli,
		kongVars,
		kong.Description(description),
	)

	err := handlergen.Run(cli.Schema, cli.Operations, cli.Out)
	k.FatalIfErrorf(err)
}
