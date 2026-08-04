module github.com/iiwish/modary/components/oidc

go 1.26.5

require (
	github.com/coreos/go-oidc/v3 v3.20.0
	github.com/go-jose/go-jose/v4 v4.1.4
	github.com/iiwish/modary v0.3.0-alpha.1
	golang.org/x/oauth2 v0.36.0
)

require (
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	golang.org/x/mod v0.38.0 // indirect
)

replace github.com/iiwish/modary => ../..
