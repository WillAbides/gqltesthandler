module github.com/willabides/gqltesthandler/example/userapi

go 1.26.1

require github.com/willabides/gqltesthandler v0.0.0 // indirect

require (
	github.com/Khan/genqlient v0.8.1
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/alecthomas/kong v1.13.0 // indirect
	github.com/alexflint/go-arg v1.6.1 // indirect
	github.com/alexflint/go-scalar v1.2.0 // indirect
	github.com/bmatcuk/doublestar/v4 v4.10.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/vektah/gqlparser/v2 v2.5.32 // indirect
	golang.org/x/mod v0.34.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/tools v0.43.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	mvdan.cc/gofumpt v0.9.2 // indirect
)

replace github.com/willabides/gqltesthandler => ../..

tool (
	github.com/Khan/genqlient
	github.com/willabides/gqltesthandler/cmd/gqltesthandler
)
