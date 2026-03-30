package userapi

//go:generate go run github.com/Khan/genqlient genqlient.yaml
//go:generate go tool gqltesthandler --schema=schema.graphqls --operations=operations.graphql -o usertest
