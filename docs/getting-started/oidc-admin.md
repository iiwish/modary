# OIDC Admin

OIDC is an explicit Admin creation choice. It replaces local password login;
it does not run beside a hidden password route and does not change business
Modules, authorization, product scope, or the React work surface.

## Create

```bash
go run github.com/iiwish/modary/cmd/modary@v0.3.0-alpha.1 \
  new operations-admin --profile admin --with oidc \
  --module example.com/acme/operations-admin
cd operations-admin
go mod tidy
```

Combine `--with oidc` with `tasks`, `audit`, or `otel` by repeating `--with`.
Fresh generated source contains `components/oidc` and the OIDC HTTP
contribution. It contains no password credential configuration, password login
route, password form, or password authenticator capability.

## Provider Registration

Register one exact redirect URI:

```text
https://admin.example.com/api/auth/oidc/callback
```

The provider must support OpenID Connect discovery and Authorization Code flow.
Modary always uses state, nonce, and PKCE S256 and verifies issuer, signature,
audience, and token time claims. HTTP issuer and redirect URLs are rejected
unless the generated local-only insecure option is explicitly enabled.

## Principal Mapping

The generated sample maps one exact provider subject to the provisioned
`admin` principal. Subject is stable only within its issuer. Email, display
name, group, role, tenant, and scope claims do not grant Modary authority.

For a real product, replace the sample mapping with reviewed provisioning that
stores exact issuer/subject-to-principal bindings. RBAC separately binds that
principal to each product scope. One principal can hold zero, one, or many
scope bindings; an unmapped subject or unbound scope fails closed.

## Configuration

```bash
export DATABASE_URL='postgres://app:...@db:5432/app?sslmode=require'
export MODARY_OIDC_ISSUER_URL='https://id.example.com'
export MODARY_OIDC_CLIENT_ID='operations-admin'
export MODARY_OIDC_CLIENT_SECRET='...'
export MODARY_OIDC_REDIRECT_URL='https://admin.example.com/api/auth/oidc/callback'
export MODARY_OIDC_SUBJECT='provider-stable-subject'
```

Client secret may be empty only for a provider configuration that deliberately
uses a public client. Deliver secrets through the deployment secret mechanism;
never place them in source, image layers, Compose files, command arguments, or
logs.

## Session And Logout

The callback creates the same server-side, revocable application session used
by protected Admin routes. Cookies are host-only, HttpOnly, SameSite, and Secure
by default. Application logout revokes the Modary session; upstream single
logout is provider-specific and outside this component.

Pending redirect ceremonies are bounded and process-local in this Alpha. Route
the callback to the instance that began login through one login instance or
ingress session affinity. Established application sessions are stored in
PostgreSQL and may be resolved by any application instance. A shared ceremony
store is not part of v0.3.

## Acceptance

Before public exposure, test login, current session, logout, application-session
revocation, restart, multiple scope bindings, and denial of an unbound scope.
Also test changed state, nonce, issuer, audience, expiry, replay, duplicate query
parameters, oversized provider responses, discovery/JWKS failure, and unmapped
subjects. MFA, recovery, enrollment, directory sync, and upstream abuse controls
remain provider and product responsibilities.
