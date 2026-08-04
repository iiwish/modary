module github.com/iiwish/modary/components/postgres

go 1.26.5

require (
	github.com/iiwish/modary v0.3.0-alpha.1
	github.com/jackc/pgx/v5 v5.10.0
	golang.org/x/crypto v0.54.0
	golang.org/x/mod v0.38.0
)

replace github.com/iiwish/modary => ../..

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)
