package userapi

//go:generate go run github.com/gqlgo/gqlgenc
//go:generate go tool gqltesthandler --schema=schema.graphqls --operations=operations.graphql -o usertest
