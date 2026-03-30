module github.com/willabides/gqltesthandler/example/github

go 1.26.1

require (
	github.com/gqlgo/gqlgenc v0.33.1
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/99designs/gqlgen v0.17.73 // indirect
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/alecthomas/kong v1.13.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.6 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/goccy/go-yaml v1.17.1 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sosodev/duration v1.3.1 // indirect
	github.com/urfave/cli/v2 v2.27.6 // indirect
	github.com/vektah/gqlparser/v2 v2.5.27 // indirect
	github.com/willabides/gqltesthandler v0.0.0 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	golang.org/x/mod v0.32.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	golang.org/x/tools v0.41.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	mvdan.cc/gofumpt v0.9.2 // indirect
)

replace github.com/willabides/gqltesthandler => ../..

tool (
	github.com/gqlgo/gqlgenc
	github.com/willabides/gqltesthandler/cmd/gqltesthandler
)
