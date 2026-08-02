package localidentity

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiwish/modary/adapters/internal/postgrestest"
	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/databasecontrol"
	"github.com/iiwish/modary/scope"
)

const (
	testPassword                        = "correct horse battery staple"
	testToken                           = "token_0123456789abcdef0123456789abcdef0123456789abcdef"
	localIdentitySynchronizationTimeout = 10 * time.Second
)

func TestEmptyProvisioningCreatesOnlySchema(t *testing.T) {
	db, service := openService(t, Options{})
	if service == nil {
		t.Fatal("service is nil")
	}
	for _, table := range []string{
		"modary_identity_principal",
		"modary_identity_password",
		"modary_identity_bearer",
		"modary_identity_session",
	} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s contains %d rows after empty provisioning", table, count)
		}
	}
	if err := service.provision(context.Background(), Options{SessionTTL: DefaultSessionTTL}); err != nil {
		t.Fatalf("repeat empty provisioning: %v", err)
	}
}

func TestCredentiallessPrincipalCanBeResolvedWithoutLoginSurface(t *testing.T) {
	options := Options{Principals: []Principal{{
		ActorID: "worker-one", ActorType: "service", DisplayName: "Worker One",
		Scope: scope.Must("account", "account-1"),
	}}}
	db, service := openService(t, options)
	actor, err := service.ResolveByID(context.Background(), "worker-one")
	if err != nil {
		t.Fatal(err)
	}
	if actor.ID != "worker-one" || actor.Type != "service" || actor.Scope != options.Principals[0].Scope {
		t.Fatalf("credentialless actor = %#v", actor)
	}
	if _, err := service.Login(context.Background(), "worker-one", testPassword); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("credentialless Login error = %v", err)
	}
	var passwordCredentials int
	if err := db.QueryRow(`SELECT COUNT(*) FROM modary_identity_password`).Scan(&passwordCredentials); err != nil {
		t.Fatal(err)
	}
	if passwordCredentials != 0 {
		t.Fatalf("credentialless principal persisted %d password credential(s)", passwordCredentials)
	}
}

func TestCredentialProvisioningRequiresAnActivePrincipal(t *testing.T) {
	db, service := openService(t, Options{})
	password := "unknown principal password"
	err := service.provision(context.Background(), Options{
		SessionTTL: DefaultSessionTTL,
		PasswordCredentials: []PasswordCredential{{
			ActorID: "missing-one", Username: "missing@example.test", Password: password,
		}},
	})
	if err == nil || strings.Contains(err.Error(), password) {
		t.Fatalf("unknown password principal error = %v", err)
	}
	token := "token_abcdef0123456789abcdef0123456789abcdef0123456789"
	err = service.provision(context.Background(), Options{
		SessionTTL:   DefaultSessionTTL,
		BearerTokens: []BearerToken{{TokenID: "missing-token", ActorID: "missing-one", Token: token}},
	})
	if err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("unknown bearer principal error = %v", err)
	}
	var credentials int
	if err := db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM modary_identity_password) +
		(SELECT COUNT(*) FROM modary_identity_bearer)`).Scan(&credentials); err != nil {
		t.Fatal(err)
	}
	if credentials != 0 {
		t.Fatalf("invalid credential provisioning persisted %d row(s)", credentials)
	}

	principal := Principal{
		ActorID: "retired-one", ActorType: "service", DisplayName: "Retired One",
		Scope: scope.Must("account", "account-1"),
	}
	if err := service.provision(context.Background(), Options{SessionTTL: DefaultSessionTTL, Principals: []Principal{principal}}); err != nil {
		t.Fatal(err)
	}
	if err := service.provision(context.Background(), Options{SessionTTL: DefaultSessionTTL, RevokedActorIDs: []string{principal.ActorID}}); err != nil {
		t.Fatal(err)
	}
	err = service.provision(context.Background(), Options{
		SessionTTL:   DefaultSessionTTL,
		BearerTokens: []BearerToken{{TokenID: "retired-token", ActorID: principal.ActorID, Token: token}},
	})
	if err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("inactive bearer principal error = %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM modary_identity_bearer`).Scan(&credentials); err != nil {
		t.Fatal(err)
	}
	if credentials != 0 {
		t.Fatalf("inactive principal received %d bearer credential(s)", credentials)
	}
}

func TestExplicitCredentialsAuthenticateAndPersistOnlyHashes(t *testing.T) {
	now := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	options := provisionedOptions(now)
	db, service := openService(t, options)
	service.clock = func() time.Time { return now }

	actor, err := service.ResolveByID(context.Background(), "person-one")
	if err != nil {
		t.Fatal(err)
	}
	if actor.ID != "person-one" || actor.Type != "human" || actor.DisplayName != "Person One" || actor.Scope != scope.Must("account", "account-1") {
		t.Fatalf("actor = %#v", actor)
	}
	if _, err := service.ResolveByID(context.Background(), "missing"); !errors.Is(err, ErrActorNotFound) {
		t.Fatalf("missing actor error = %v", err)
	}
	if _, err := service.Login(context.Background(), "person@example.test", "wrong password value"); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("wrong password error = %v", err)
	}

	session, err := service.Login(context.Background(), "person@example.test", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if session.Token == "" || session.CSRFToken == "" || session.Actor != actor || !session.ExpiresAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("session = %#v", session)
	}
	loaded, err := service.Session(context.Background(), session.Token)
	if err != nil || loaded != session {
		t.Fatalf("Session() = %#v, %v", loaded, err)
	}
	bearerActor, err := service.AuthenticateToken(context.Background(), testToken)
	if err != nil || bearerActor != actor {
		t.Fatalf("AuthenticateToken() = %#v, %v", bearerActor, err)
	}
	if _, err := service.AuthenticateToken(context.Background(), strings.Repeat("x", 40)); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("unknown token error = %v", err)
	}

	var encodedPassword, encodedBearer, encodedSession string
	if err := db.QueryRow(`SELECT password_hash FROM modary_identity_password`).Scan(&encodedPassword); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT token_hash FROM modary_identity_bearer`).Scan(&encodedBearer); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT token_hash FROM modary_identity_session`).Scan(&encodedSession); err != nil {
		t.Fatal(err)
	}
	for name, encoded := range map[string]string{"password": encodedPassword, "bearer": encodedBearer, "session": encodedSession} {
		if encoded == testPassword || encoded == testToken || encoded == session.Token || strings.Contains(encoded, testPassword) || strings.Contains(encoded, testToken) {
			t.Fatalf("%s was stored in plaintext: %q", name, encoded)
		}
	}
	if !strings.HasPrefix(encodedPassword, "$argon2id$v=19$") || !strings.HasPrefix(encodedBearer, "sha256:") || !strings.HasPrefix(encodedSession, "sha256:") {
		t.Fatalf("stored hashes = %q, %q, %q", encodedPassword, encodedBearer, encodedSession)
	}
	if len(encodedPassword) != passwordHashEncodedBytes {
		t.Fatalf("stored password hash length = %d, want %d", len(encodedPassword), passwordHashEncodedBytes)
	}
}

func TestOpaqueActorIdentifiersFollowTheKernelContract(t *testing.T) {
	executionScope := scope.Must("tenant", "tenant-one")
	options := Options{
		Principals: []Principal{{
			ActorID:     "01JABCDEF|user@example.test",
			ActorType:   "外部身份/service",
			DisplayName: "External User",
			Scope:       executionScope,
		}},
		PasswordCredentials: []PasswordCredential{{
			ActorID:  "01JABCDEF|user@example.test",
			Username: "external@example.test",
			Password: testPassword,
		}},
	}
	_, service := openService(t, options)
	actor, err := service.ResolveByID(context.Background(), options.Principals[0].ActorID)
	if err != nil {
		t.Fatal(err)
	}
	if actor.ID != options.Principals[0].ActorID || actor.Type != options.Principals[0].ActorType || actor.Scope != executionScope {
		t.Fatalf("resolved actor = %#v", actor)
	}
}

func TestOptionalDisplayNameRoundTripsThroughCredentials(t *testing.T) {
	options := provisionedOptions(time.Now().UTC())
	options.Principals[0].DisplayName = ""
	_, service := openService(t, options)
	actor, err := service.ResolveByID(context.Background(), "person-one")
	if err != nil || actor.DisplayName != "" {
		t.Fatalf("resolved actor = %#v, %v", actor, err)
	}
	session, err := service.Login(context.Background(), "person@example.test", testPassword)
	if err != nil || session.Actor.DisplayName != "" {
		t.Fatalf("session actor = %#v, %v", session.Actor, err)
	}
	bearer, err := service.AuthenticateToken(context.Background(), testToken)
	if err != nil || bearer.DisplayName != "" {
		t.Fatalf("bearer actor = %#v, %v", bearer, err)
	}
}

func TestProvisioningIsIdempotentAndSupportsRotationAndRevocation(t *testing.T) {
	now := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	options := provisionedOptions(now)
	db, service := openService(t, options)
	service.clock = func() time.Time { return now }
	session, err := service.Login(context.Background(), "person@example.test", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	var originalHash string
	if err := db.QueryRow(`SELECT password_hash FROM modary_identity_password`).Scan(&originalHash); err != nil {
		t.Fatal(err)
	}
	options.Random = errorReader{err: errors.New("random must not be used for unchanged password")}
	restarted := newService(identityControl(t, db), options)
	restarted.clock = func() time.Time { return now }
	if err := restarted.provision(context.Background(), options); err != nil {
		t.Fatalf("idempotent provisioning: %v", err)
	}
	var repeatedHash string
	if err := db.QueryRow(`SELECT password_hash FROM modary_identity_password`).Scan(&repeatedHash); err != nil || repeatedHash != originalHash {
		t.Fatalf("repeated hash = %q, %v", repeatedHash, err)
	}
	if _, err := restarted.Session(context.Background(), session.Token); err != nil {
		t.Fatalf("unchanged provisioning invalidated session: %v", err)
	}

	rotatedPassword := "a newly rotated password"
	rotatedToken := "token_abcdef0123456789abcdef0123456789abcdef0123456789"
	rotated := provisionedOptions(now.Add(time.Hour))
	rotated.PasswordCredentials[0].Password = rotatedPassword
	rotated.BearerTokens[0].Token = rotatedToken
	rotated.Random = bytes.NewReader(bytes.Repeat([]byte{0x4a}, 256))
	rotatedService := newService(identityControl(t, db), rotated)
	rotatedService.clock = func() time.Time { return now.Add(time.Hour) }
	if err := rotatedService.provision(context.Background(), rotated); err != nil {
		t.Fatal(err)
	}
	if _, err := rotatedService.Session(context.Background(), session.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("password rotation retained session: %v", err)
	}
	if _, err := rotatedService.Login(context.Background(), "person@example.test", testPassword); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("old password error = %v", err)
	}
	if _, err := rotatedService.Login(context.Background(), "person@example.test", rotatedPassword); err != nil {
		t.Fatalf("rotated password: %v", err)
	}
	if _, err := rotatedService.AuthenticateToken(context.Background(), testToken); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("old token error = %v", err)
	}
	if _, err := rotatedService.AuthenticateToken(context.Background(), rotatedToken); err != nil {
		t.Fatalf("rotated token: %v", err)
	}

	revoked := Options{
		SessionTTL:      DefaultSessionTTL,
		RevokedActorIDs: []string{"person-one"},
		RevokedTokenIDs: []string{"automation-one"},
	}
	revokedService := newService(identityControl(t, db), revoked)
	if err := revokedService.provision(context.Background(), revoked); err != nil {
		t.Fatal(err)
	}
	if _, err := revokedService.ResolveByID(context.Background(), "person-one"); !errors.Is(err, ErrActorNotFound) {
		t.Fatalf("revoked actor error = %v", err)
	}
	if _, err := revokedService.AuthenticateToken(context.Background(), rotatedToken); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("revoked token error = %v", err)
	}
	if err := revokedService.provision(context.Background(), revoked); err != nil {
		t.Fatalf("repeat revocation: %v", err)
	}

	reactivated := rotated
	reactivated.BearerTokens = nil
	if err := revokedService.provision(context.Background(), reactivated); err != nil {
		t.Fatalf("reactivate user: %v", err)
	}
	if _, err := revokedService.ResolveByID(context.Background(), "person-one"); err != nil {
		t.Fatalf("reactivated actor: %v", err)
	}
	if _, err := revokedService.AuthenticateToken(context.Background(), rotatedToken); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("actor reactivation resurrected bearer token: %v", err)
	}
	if err := revokedService.provision(context.Background(), Options{
		BearerTokens: []BearerToken{{TokenID: "automation-one", ActorID: "person-one", Token: rotatedToken}},
	}); err != nil {
		t.Fatalf("explicit token reactivation: %v", err)
	}
	if _, err := revokedService.AuthenticateToken(context.Background(), rotatedToken); err != nil {
		t.Fatalf("explicitly reactivated token: %v", err)
	}
}

func TestProvisioningRollsBackAsOneTransaction(t *testing.T) {
	now := time.Now().UTC()
	options := provisionedOptions(now)
	options.BearerTokens[0].ActorID = "missing-actor"
	db := openIdentityDatabase(t)
	service := newService(identityControl(t, db), options)
	if err := service.provision(context.Background(), options); err == nil {
		t.Fatal("provisioning with missing token actor succeeded")
	}
	for _, table := range []string{"modary_identity_principal", "modary_identity_password", "modary_identity_bearer"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count = %d, %v", table, count, err)
		}
	}
}

func TestSessionsExpireAndLogoutIsIdempotent(t *testing.T) {
	now := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	options := provisionedOptions(now)
	_, service := openService(t, options)
	service.clock = func() time.Time { return now }
	session, err := service.Login(context.Background(), "person@example.test", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(options.SessionTTL)
	if _, err := service.Session(context.Background(), session.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("expired session error = %v", err)
	}
	if err := service.Logout(context.Background(), session.Token); err != nil {
		t.Fatal(err)
	}
	if err := service.Logout(context.Background(), session.Token); err != nil {
		t.Fatalf("repeat Logout() error = %v", err)
	}
}

func TestLoginRemovesExpiredSessions(t *testing.T) {
	now := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	options := provisionedOptions(now)
	options.Random = nil
	db, service := openService(t, options)
	service.clock = func() time.Time { return now }
	first, err := service.Login(context.Background(), "person@example.test", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(options.SessionTTL)
	if _, err := service.Login(context.Background(), "person@example.test", testPassword); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM modary_identity_session`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("session count after cleanup = %d, want 1", count)
	}
	if _, err := service.Session(context.Background(), first.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("expired session error = %v", err)
	}
}

func TestSecurityContextChangeInvalidatesExistingCredentials(t *testing.T) {
	options := provisionedOptions(time.Now().UTC())
	db, service := openService(t, options)
	session, err := service.Login(context.Background(), "person@example.test", testPassword)
	if err != nil {
		t.Fatal(err)
	}

	changed := options
	changed.Principals = append([]Principal(nil), options.Principals...)
	changed.Principals[0].ActorType = "service"
	changed.Principals[0].Scope = scope.Must("account", "account-2")
	changed.BearerTokens = nil
	changed.Random = errorReader{err: errors.New("unchanged password must not consume random")}
	changedService := newService(identityControl(t, db), changed)
	if err := changedService.provision(context.Background(), changed); err != nil {
		t.Fatal(err)
	}
	if _, err := changedService.Session(context.Background(), session.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("security-context change retained session: %v", err)
	}
	if _, err := changedService.AuthenticateToken(context.Background(), testToken); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("security-context change retained bearer: %v", err)
	}
	actor, err := changedService.ResolveByID(context.Background(), "person-one")
	if err != nil || actor.Type != "service" || actor.Scope != changed.Principals[0].Scope {
		t.Fatalf("updated actor = %#v, %v", actor, err)
	}

	changed.BearerTokens = options.BearerTokens
	if err := changedService.provision(context.Background(), changed); err != nil {
		t.Fatal(err)
	}
	if actor, err := changedService.AuthenticateToken(context.Background(), testToken); err != nil || actor.Scope != changed.Principals[0].Scope {
		t.Fatalf("explicitly reactivated bearer = %#v, %v", actor, err)
	}
}

func TestCredentialReadsClassifyConcurrentRevocation(t *testing.T) {
	options := provisionedOptions(time.Now().UTC())
	db, service := openService(t, options)
	session, err := service.Login(context.Background(), "person@example.test", testPassword)
	if err != nil {
		t.Fatal(err)
	}

	writerDone := make(chan error, 1)
	go func() {
		for index := range 100 {
			active := index%2 == 1
			if _, err := db.Exec(`UPDATE modary_identity_principal SET active = $1 WHERE actor_id = 'person-one'`, active); err != nil {
				writerDone <- err
				return
			}
		}
		_, err := db.Exec(`UPDATE modary_identity_principal SET active = FALSE WHERE actor_id = 'person-one'`)
		writerDone <- err
	}()
	for range 200 {
		if _, err := service.Session(context.Background(), session.Token); err != nil && !errors.Is(err, ErrSessionInvalid) {
			t.Fatalf("concurrent Session error = %v", err)
		}
		if _, err := service.AuthenticateToken(context.Background(), testToken); err != nil && !errors.Is(err, ErrAuthenticationFailed) {
			t.Fatalf("concurrent AuthenticateToken error = %v", err)
		}
	}
	if err := <-writerDone; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Session(context.Background(), session.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("revoked Session error = %v", err)
	}
	if _, err := service.AuthenticateToken(context.Background(), testToken); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("revoked AuthenticateToken error = %v", err)
	}
}

func TestGenerateBearerTokenProducesAcceptedHighEntropyRepresentation(t *testing.T) {
	first, err := GenerateBearerToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateBearerToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || len(first) != 43 || strings.ContainsAny(first, "+/=") {
		t.Fatalf("generated bearer representations = %q, %q", first, second)
	}
	options := provisionedOptions(time.Now().UTC())
	options.BearerTokens[0].Token = first
	if _, err := normalizeOptions(options); err != nil {
		t.Fatalf("generated bearer token was rejected: %v", err)
	}
}

func TestConcurrentLoginProducesDistinctSessions(t *testing.T) {
	now := time.Now().UTC()
	options := provisionedOptions(now)
	options.Random = nil
	_, service := openService(t, options)
	const workers = 8
	tokens := make(chan string, workers)
	errorsSeen := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			session, err := service.Login(context.Background(), "person@example.test", testPassword)
			if err != nil {
				errorsSeen <- err
				return
			}
			tokens <- session.Token
		}()
	}
	group.Wait()
	close(tokens)
	close(errorsSeen)
	for err := range errorsSeen {
		t.Errorf("concurrent Login() error = %v", err)
	}
	unique := make(map[string]struct{})
	for token := range tokens {
		unique[token] = struct{}{}
	}
	if len(unique) != workers {
		t.Fatalf("unique session count = %d, want %d", len(unique), workers)
	}
}

func TestPasswordRotationCannotRaceAStaleLoginIntoAValidSession(t *testing.T) {
	db, loginService := openService(t, provisionedOptions(time.Now().UTC()))
	barrier := &blockingRandom{entered: make(chan struct{}), release: make(chan struct{})}
	loginService.random = barrier
	loginService.passwordVerifier = func(encoded, password string) bool {
		return encoded != "" && password == testPassword
	}

	loginResult := make(chan error, 1)
	go func() {
		_, err := loginService.Login(context.Background(), "person@example.test", testPassword)
		loginResult <- err
	}()
	select {
	case <-barrier.entered:
	case <-time.After(localIdentitySynchronizationTimeout):
		t.Fatal("Login did not reach post-verification random generation")
	}

	rotated := provisionedOptions(time.Now().UTC())
	rotated.PasswordCredentials[0].Password = "a newly rotated password"
	rotated.BearerTokens = nil
	rotated.Random = bytes.NewReader(bytes.Repeat([]byte{0x7c}, 128))
	normalized, err := normalizeOptions(rotated)
	if err != nil {
		t.Fatal(err)
	}
	rotationService := newService(identityControl(t, db), normalized)
	if err := rotationService.provision(context.Background(), normalized); err != nil {
		t.Fatal(err)
	}
	close(barrier.release)
	if err := <-loginResult; !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("stale Login error = %v", err)
	}
	var sessions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM modary_identity_session`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Fatalf("stale Login persisted %d session(s)", sessions)
	}
}

func TestOptionsFailClosedBeforeRegistration(t *testing.T) {
	valid := provisionedOptions(time.Now().UTC())
	var typedNil *bytes.Reader
	tests := []struct {
		name   string
		mutate func(*Options)
	}{
		{name: "negative TTL", mutate: func(options *Options) { options.SessionTTL = -time.Second }},
		{name: "too short TTL", mutate: func(options *Options) { options.SessionTTL = time.Second }},
		{name: "negative password concurrency", mutate: func(options *Options) { options.MaxConcurrentPasswordChecks = -1 }},
		{name: "excessive password concurrency", mutate: func(options *Options) { options.MaxConcurrentPasswordChecks = MaximumConcurrentPasswordChecks + 1 }},
		{name: "typed nil random", mutate: func(options *Options) { options.Random = typedNil }},
		{name: "duplicate actor", mutate: func(options *Options) { options.Principals = append(options.Principals, options.Principals[0]) }},
		{name: "duplicate username", mutate: func(options *Options) {
			duplicate := options.PasswordCredentials[0]
			duplicate.ActorID = "person-two"
			options.PasswordCredentials = append(options.PasswordCredentials, duplicate)
		}},
		{name: "duplicate password actor", mutate: func(options *Options) {
			duplicate := options.PasswordCredentials[0]
			duplicate.Username = "person-two@example.test"
			options.PasswordCredentials = append(options.PasswordCredentials, duplicate)
		}},
		{name: "short password", mutate: func(options *Options) { options.PasswordCredentials[0].Password = "too-short" }},
		{name: "invalid scope", mutate: func(options *Options) { options.Principals[0].Scope = scope.Execution{} }},
		{name: "short bearer", mutate: func(options *Options) { options.BearerTokens[0].Token = "short" }},
		{name: "internal bearer whitespace", mutate: func(options *Options) {
			options.BearerTokens[0].Token = strings.Repeat("a", 16) + " " + strings.Repeat("b", 16)
		}},
		{name: "unicode bearer whitespace", mutate: func(options *Options) {
			options.BearerTokens[0].Token = strings.Repeat("a", 16) + "\u00a0" + strings.Repeat("b", 16)
		}},
		{name: "invalid UTF-8 bearer", mutate: func(options *Options) {
			options.BearerTokens[0].Token = strings.Repeat("a", 32) + string([]byte{0xff})
		}},
		{name: "duplicate token", mutate: func(options *Options) { options.BearerTokens = append(options.BearerTokens, options.BearerTokens[0]) }},
		{name: "provision and revoke actor", mutate: func(options *Options) { options.RevokedActorIDs = []string{"person-one"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := valid
			options.Principals = append([]Principal(nil), valid.Principals...)
			options.PasswordCredentials = append([]PasswordCredential(nil), valid.PasswordCredentials...)
			options.BearerTokens = append([]BearerToken(nil), valid.BearerTokens...)
			test.mutate(&options)
			registration, err := Module(options)
			if err == nil || registration.Definition.Manifest.ID != "" {
				t.Fatalf("Module() = %#v, %v", registration, err)
			}
		})
	}

	registration, err := Module(Options{})
	if err != nil {
		t.Fatalf("Module(empty) error = %v", err)
	}
	if registration.Definition.Manifest.ID != ModuleID || registration.Start == nil || len(registration.Definition.Migrations) != 1 {
		t.Fatalf("registration = %#v", registration.Definition)
	}
}

func TestOptionsAreDefensivelyCopiedAcrossPrincipalAndCredentialBoundaries(t *testing.T) {
	source := provisionedOptions(time.Now().UTC())
	source.RevokedActorIDs = []string{"retired-one"}
	source.RevokedTokenIDs = []string{"retired-token"}
	normalized, err := normalizeOptions(source)
	if err != nil {
		t.Fatal(err)
	}
	source.Principals[0].ActorID = "mutated-principal"
	source.PasswordCredentials[0].Username = "mutated@example.test"
	source.BearerTokens[0].TokenID = "mutated-token"
	source.RevokedActorIDs[0] = "mutated-revocation"
	source.RevokedTokenIDs[0] = "mutated-token-revocation"

	if normalized.Principals[0].ActorID != "person-one" ||
		normalized.PasswordCredentials[0].Username != "person@example.test" ||
		normalized.BearerTokens[0].TokenID != "automation-one" ||
		normalized.RevokedActorIDs[0] != "retired-one" || normalized.RevokedTokenIDs[0] != "retired-token" {
		t.Fatalf("normalized options changed with source mutation: %#v", normalized)
	}
}

func TestPasswordVerificationConcurrencyIsBoundedAndCancelable(t *testing.T) {
	options := provisionedOptions(time.Now().UTC())
	options.MaxConcurrentPasswordChecks = 1
	_, service := openService(t, options)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var active atomic.Int32
	var maximum atomic.Int32
	service.passwordVerifier = func(string, string) bool {
		current := active.Add(1)
		for previous := maximum.Load(); current > previous && !maximum.CompareAndSwap(previous, current); previous = maximum.Load() {
		}
		entered <- struct{}{}
		<-release
		active.Add(-1)
		return false
	}

	firstResult := make(chan error, 1)
	go func() {
		_, err := service.Login(context.Background(), "person@example.test", "wrong password value")
		firstResult <- err
	}()
	select {
	case <-entered:
	case <-time.After(localIdentitySynchronizationTimeout):
		t.Fatal("first password check did not start")
	}

	secondContext, cancel := context.WithCancel(context.Background())
	secondResult := make(chan error, 1)
	go func() {
		_, err := service.Login(secondContext, "missing@example.test", "wrong password value")
		secondResult <- err
	}()
	select {
	case <-entered:
		close(release)
		t.Fatal("second password check bypassed the concurrency bound")
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	if err := <-secondResult; !errors.Is(err, context.Canceled) {
		close(release)
		t.Fatalf("canceled password wait error = %v", err)
	}
	close(release)
	if err := <-firstResult; !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("first password result = %v", err)
	}
	if maximum.Load() != 1 {
		t.Fatalf("maximum concurrent password checks = %d", maximum.Load())
	}
}

func TestRandomFailuresAndPanicsAreContained(t *testing.T) {
	for _, test := range []struct {
		name   string
		reader io.Reader
		want   error
	}{
		{name: "error", reader: errorReader{err: io.ErrUnexpectedEOF}, want: io.ErrUnexpectedEOF},
		{name: "panic", reader: panicReader{}, want: ErrRandomSourcePanic},
	} {
		t.Run(test.name, func(t *testing.T) {
			options, err := normalizeOptions(provisionedOptions(time.Now()))
			if err != nil {
				t.Fatal(err)
			}
			options.Random = test.reader
			service := newService(identityControl(t, openIdentityDatabase(t)), options)
			err = service.provision(context.Background(), options)
			if !errors.Is(err, test.want) {
				t.Fatalf("provision error = %v, want errors.Is(%v)", err, test.want)
			}
		})
	}
}

func TestLoginRejectsMalformedInputsAndCorruptCredentials(t *testing.T) {
	db, service := openService(t, provisionedOptions(time.Now()))
	if _, err := service.Login(context.Background(), " bad ", testPassword); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("invalid username error = %v", err)
	}
	if _, err := service.Login(context.Background(), "person@example.test", strings.Repeat("x", 4097)); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("oversized password error = %v", err)
	}
	malformedHash := strings.Repeat("x", passwordHashEncodedBytes)
	if _, err := db.Exec(`UPDATE modary_identity_password SET password_hash = $1`, malformedHash); err != nil {
		t.Fatal(err)
	}
	_, err := service.Login(context.Background(), "person@example.test", testPassword)
	if err == nil || errors.Is(err, ErrAuthenticationFailed) || strings.Contains(err.Error(), "corrupt-secret-material") {
		t.Fatalf("corrupt credential error = %v", err)
	}
}

func TestPostgresSchemaRejectsOversizedIdentityFields(t *testing.T) {
	t.Run("password login", func(t *testing.T) {
		db, _ := openService(t, provisionedOptions(time.Now()))
		oversized := strings.Repeat("p", passwordHashEncodedBytes+1)
		if _, err := db.Exec(`UPDATE modary_identity_password SET password_hash = $1`, oversized); err == nil {
			t.Fatal("oversized password hash passed the PostgreSQL constraint")
		}
	})

	t.Run("actor resolution", func(t *testing.T) {
		db, _ := openService(t, provisionedOptions(time.Now()))
		oversized := strings.Repeat("d", maxStoredDisplayNameBytes+1)
		if _, err := db.Exec(`UPDATE modary_identity_principal SET display_name = $1`, oversized); err == nil {
			t.Fatal("oversized display name passed the PostgreSQL constraint")
		}
	})

	t.Run("session", func(t *testing.T) {
		db, service := openService(t, provisionedOptions(time.Now()))
		if _, err := service.Login(context.Background(), "person@example.test", testPassword); err != nil {
			t.Fatal(err)
		}
		oversized := strings.Repeat("c", maxStoredCSRFTokenBytes+1)
		if _, err := db.Exec(`UPDATE modary_identity_session SET csrf_token = $1`, oversized); err == nil {
			t.Fatal("oversized CSRF token passed the PostgreSQL constraint")
		}
	})

	t.Run("bearer actor", func(t *testing.T) {
		db, _ := openService(t, provisionedOptions(time.Now()))
		oversized := strings.Repeat("t", maxStoredActorTypeBytes+1)
		if _, err := db.Exec(`UPDATE modary_identity_principal SET actor_type = $1`, oversized); err == nil {
			t.Fatal("oversized actor type passed the PostgreSQL constraint")
		}
	})

	t.Run("provisioning re-read", func(t *testing.T) {
		options := provisionedOptions(time.Now())
		db, _ := openService(t, options)
		oversized := strings.Repeat("s", maxStoredScopeIDBytes+1)
		if _, err := db.Exec(`UPDATE modary_identity_principal SET scope_id = $1`, oversized); err == nil {
			t.Fatal("oversized scope id passed the PostgreSQL constraint")
		}
	})
}

func TestPasswordHashParserRejectsNonCanonicalEncodedLength(t *testing.T) {
	for _, encoded := range []string{
		strings.Repeat("x", passwordHashEncodedBytes-1),
		strings.Repeat("x", passwordHashEncodedBytes+1),
		strings.Repeat("x", 1<<20),
	} {
		if _, _, _, _, _, ok := parsePasswordHash(encoded); ok {
			t.Fatalf("parsePasswordHash accepted %d-byte encoding", len(encoded))
		}
	}
}

func TestPublicMethodsRejectNilContexts(t *testing.T) {
	_, service := openService(t, provisionedOptions(time.Now().UTC()))
	if _, err := service.ResolveByID(nil, "person-one"); !errors.Is(err, ErrContextRequired) {
		t.Errorf("ResolveByID(nil) error = %v", err)
	}
	if _, err := service.Login(nil, "person@example.test", testPassword); !errors.Is(err, ErrContextRequired) {
		t.Errorf("Login(nil) error = %v", err)
	}
	if err := service.Logout(nil, "token"); !errors.Is(err, ErrContextRequired) {
		t.Errorf("Logout(nil) error = %v", err)
	}
	if _, err := service.Session(nil, "token"); !errors.Is(err, ErrContextRequired) {
		t.Errorf("Session(nil) error = %v", err)
	}
	if _, err := service.AuthenticateToken(nil, testToken); !errors.Is(err, ErrContextRequired) {
		t.Errorf("AuthenticateToken(nil) error = %v", err)
	}
}

func TestCredentialMethodsRejectMalformedTokensBeforeDatabaseBoundary(t *testing.T) {
	backend := &credentialBoundaryBackend{}
	control, err := databasecontrol.New(backend)
	if err != nil {
		t.Fatal(err)
	}
	service := &service{control: control}
	for _, token := range []string{
		strings.Repeat("x", 4097),
		strings.Repeat("x", 32) + "\n",
		strings.Repeat("x", 32) + "\u00a0",
		strings.Repeat("x", 32) + string([]byte{0xff}),
	} {
		if err := service.Logout(context.Background(), token); !errors.Is(err, ErrSessionInvalid) {
			t.Errorf("Logout malformed token error = %v", err)
		}
		if _, err := service.Session(context.Background(), token); !errors.Is(err, ErrSessionInvalid) {
			t.Errorf("Session malformed token error = %v", err)
		}
		if _, err := service.AuthenticateToken(context.Background(), token); !errors.Is(err, ErrAuthenticationFailed) {
			t.Errorf("AuthenticateToken malformed token error = %v", err)
		}
	}
	if err := service.Logout(context.Background(), ""); err != nil {
		t.Errorf("Logout empty token error = %v", err)
	}
	if backend.executorCalls != 0 || backend.transactionCalls != 0 {
		t.Fatalf("malformed credentials crossed database boundary: executors=%d transactions=%d", backend.executorCalls, backend.transactionCalls)
	}
}

func openService(t *testing.T, raw Options) (*sql.DB, *service) {
	t.Helper()
	options, err := normalizeOptions(raw)
	if err != nil {
		t.Fatal(err)
	}
	db := openIdentityDatabase(t)
	service := newService(identityControl(t, db), options)
	if err := service.provision(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	return db, service
}

func openIdentityDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, control := postgrestest.Open(t)
	if err := control.ApplyMigrations(context.Background(), ModuleID, postgresMigrations); err != nil {
		t.Fatal(err)
	}
	return db
}

func identityControl(t *testing.T, db *sql.DB) databasecontrol.Control {
	t.Helper()
	control, err := postgrestest.NewControl(db)
	if err != nil {
		t.Fatal(err)
	}
	return control
}

type credentialBoundaryBackend struct {
	executorCalls    int
	transactionCalls int
}

func (*credentialBoundaryBackend) Driver() string { return "credential-boundary" }

func (*credentialBoundaryBackend) ValidateMigration(string) error {
	panic("malformed credential reached migration validation")
}

func (backend *credentialBoundaryBackend) ReadExecutor(context.Context) (database.Executor, error) {
	backend.executorCalls++
	panic("malformed credential resolved a read executor")
}

func (backend *credentialBoundaryBackend) WriteExecutor(context.Context) (database.Executor, error) {
	backend.executorCalls++
	panic("malformed credential resolved a write executor")
}

func (backend *credentialBoundaryBackend) AdminExecutor(context.Context) (database.Executor, error) {
	backend.executorCalls++
	panic("malformed credential resolved an admin executor")
}

func (backend *credentialBoundaryBackend) WithinTransaction(context.Context, func(context.Context) error) error {
	backend.transactionCalls++
	panic("malformed credential opened a transaction")
}

func provisionedOptions(_ time.Time) Options {
	return Options{
		Principals: []Principal{{
			ActorID: "person-one", ActorType: "human", DisplayName: "Person One",
			Scope: scope.Must("account", "account-1"),
		}},
		PasswordCredentials: []PasswordCredential{{
			ActorID: "person-one", Username: "person@example.test", Password: testPassword,
		}},
		BearerTokens: []BearerToken{{TokenID: "automation-one", ActorID: "person-one", Token: testToken}},
		SessionTTL:   2 * time.Hour,
		Random:       bytes.NewReader(bytes.Repeat([]byte{0x3c}, 512)),
	}
}

type errorReader struct{ err error }

func (reader errorReader) Read([]byte) (int, error) { return 0, reader.err }

var _ io.Reader = errorReader{}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("random source secret panic") }

type blockingRandom struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (reader *blockingRandom) Read(target []byte) (int, error) {
	reader.once.Do(func() { close(reader.entered) })
	<-reader.release
	for index := range target {
		target[index] = 0x5a
	}
	return len(target), nil
}
