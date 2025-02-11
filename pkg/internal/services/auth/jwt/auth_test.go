package jwt

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/grafana/grafana/pkg/internal/infra/remotecache"
	"github.com/grafana/grafana/pkg/internal/registry"
	"github.com/grafana/grafana/pkg/internal/services/sqlstore"
	"github.com/grafana/grafana/pkg/internal/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	jose "gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"
)

type scenarioContext struct {
	ctx        context.Context
	cfg        *setting.Cfg
	authJWTSvc *AuthService
}

type cachingScenarioContext struct {
	scenarioContext
	reqCount *int
}

type configureFunc func(*testing.T, *setting.Cfg)
type scenarioFunc func(*testing.T, scenarioContext)
type cachingScenarioFunc func(*testing.T, cachingScenarioContext)

func TestVerifyUsingPKIXPublicKeyFile(t *testing.T) {
	subject := "foo-subj"

	key := rsaKeys[0]
	unknownKey := rsaKeys[1]

	scenario(t, "verifies a token", func(t *testing.T, sc scenarioContext) {
		token := sign(t, key, jwt.Claims{
			Subject: subject,
		})
		verifiedClaims, err := sc.authJWTSvc.Verify(sc.ctx, token)
		require.NoError(t, err)
		assert.Equal(t, verifiedClaims["sub"], subject)
	}, configurePKIXPublicKeyFile)

	scenario(t, "rejects a token signed by unknown key", func(t *testing.T, sc scenarioContext) {
		token := sign(t, unknownKey, jwt.Claims{
			Subject: subject,
		})
		_, err := sc.authJWTSvc.Verify(sc.ctx, token)
		require.Error(t, err)
	}, configurePKIXPublicKeyFile)
}

func TestVerifyUsingJWKSetFile(t *testing.T) {
	configure := func(t *testing.T, cfg *setting.Cfg) {
		t.Helper()

		file, err := ioutil.TempFile(os.TempDir(), "jwk-*.json")
		require.NoError(t, err)
		t.Cleanup(func() {
			if err := os.Remove(file.Name()); err != nil {
				panic(err)
			}
		})

		require.NoError(t, json.NewEncoder(file).Encode(jwksPublic))
		require.NoError(t, file.Close())

		cfg.JWTAuthJWKSetFile = file.Name()
	}

	subject := "foo-subj"

	scenario(t, "verifies a token signed with a key from the set", func(t *testing.T, sc scenarioContext) {
		token := sign(t, &jwKeys[0], jwt.Claims{Subject: subject})
		verifiedClaims, err := sc.authJWTSvc.Verify(sc.ctx, token)
		require.NoError(t, err)
		assert.Equal(t, verifiedClaims["sub"], subject)
	}, configure)

	scenario(t, "verifies a token signed with another key from the set", func(t *testing.T, sc scenarioContext) {
		token := sign(t, &jwKeys[1], jwt.Claims{Subject: subject})
		verifiedClaims, err := sc.authJWTSvc.Verify(sc.ctx, token)
		require.NoError(t, err)
		assert.Equal(t, verifiedClaims["sub"], subject)
	}, configure)

	scenario(t, "rejects a token signed with a key not from the set", func(t *testing.T, sc scenarioContext) {
		token := sign(t, jwKeys[2], jwt.Claims{Subject: subject})
		_, err := sc.authJWTSvc.Verify(sc.ctx, token)
		require.Error(t, err)
	}, configure)
}

func TestVerifyUsingJWKSetURL(t *testing.T) {
	subject := "foo-subj"

	t.Run("should refuse to start with non-https URL", func(t *testing.T) {
		var err error

		_, err = initAuthService(t, func(t *testing.T, cfg *setting.Cfg) {
			cfg.JWTAuthJWKSetURL = "https://example.com/.well-known/jwks.json"
		})
		require.NoError(t, err)

		_, err = initAuthService(t, func(t *testing.T, cfg *setting.Cfg) {
			cfg.JWTAuthJWKSetURL = "http://example.com/.well-known/jwks.json"
		})
		require.Error(t, err)
	})

	jwkHTTPScenario(t, "verifies a token signed with a key from the set", func(t *testing.T, sc scenarioContext) {
		token := sign(t, &jwKeys[0], jwt.Claims{Subject: subject})
		verifiedClaims, err := sc.authJWTSvc.Verify(sc.ctx, token)
		require.NoError(t, err)
		assert.Equal(t, verifiedClaims["sub"], subject)
	})

	jwkHTTPScenario(t, "verifies a token signed with another key from the set", func(t *testing.T, sc scenarioContext) {
		token := sign(t, &jwKeys[1], jwt.Claims{Subject: subject})
		verifiedClaims, err := sc.authJWTSvc.Verify(sc.ctx, token)
		require.NoError(t, err)
		assert.Equal(t, verifiedClaims["sub"], subject)
	})

	jwkHTTPScenario(t, "rejects a token signed with a key not from the set", func(t *testing.T, sc scenarioContext) {
		token := sign(t, jwKeys[2], jwt.Claims{Subject: subject})
		_, err := sc.authJWTSvc.Verify(sc.ctx, token)
		require.Error(t, err)
	})
}

func TestCachingJWKHTTPResponse(t *testing.T) {
	subject := "foo-subj"

	jwkCachingScenario(t, "caches the jwk response", func(t *testing.T, sc cachingScenarioContext) {
		for i := 0; i < 5; i++ {
			token := sign(t, &jwKeys[0], jwt.Claims{Subject: subject})
			_, err := sc.authJWTSvc.Verify(sc.ctx, token)
			require.NoError(t, err, "verify call %d", i+1)
		}

		assert.Equal(t, 1, *sc.reqCount)
	})

	jwkCachingScenario(t, "respects TTL setting", func(t *testing.T, sc cachingScenarioContext) {
		var err error

		token0 := sign(t, &jwKeys[0], jwt.Claims{Subject: subject})
		token1 := sign(t, &jwKeys[1], jwt.Claims{Subject: subject})

		_, err = sc.authJWTSvc.Verify(sc.ctx, token0)
		require.NoError(t, err)
		_, err = sc.authJWTSvc.Verify(sc.ctx, token1)
		require.Error(t, err)

		assert.Equal(t, 1, *sc.reqCount)

		time.Sleep(sc.cfg.JWTAuthCacheTTL + time.Millisecond)

		_, err = sc.authJWTSvc.Verify(sc.ctx, token1)
		require.NoError(t, err)
		_, err = sc.authJWTSvc.Verify(sc.ctx, token0)
		require.Error(t, err)

		assert.Equal(t, 2, *sc.reqCount)
	}, func(t *testing.T, cfg *setting.Cfg) {
		cfg.JWTAuthCacheTTL = time.Second
	})

	jwkCachingScenario(t, "does not cache the response when TTL is zero", func(t *testing.T, sc cachingScenarioContext) {
		for i := 0; i < 2; i++ {
			_, err := sc.authJWTSvc.Verify(sc.ctx, sign(t, &jwKeys[i], jwt.Claims{Subject: subject}))
			require.NoError(t, err, "verify call %d", i+1)
		}

		assert.Equal(t, 2, *sc.reqCount)
	}, func(t *testing.T, cfg *setting.Cfg) {
		cfg.JWTAuthCacheTTL = 0
	})
}

func TestSignatureWithNoneAlgorithm(t *testing.T) {
	scenario(t, "rejects a token signed with \"none\" algorithm", func(t *testing.T, sc scenarioContext) {
		token := signNone(t, jwt.Claims{Subject: "foo"})
		_, err := sc.authJWTSvc.Verify(sc.ctx, token)
		require.Error(t, err)
	}, configurePKIXPublicKeyFile)
}

func TestClaimValidation(t *testing.T) {
	key := rsaKeys[0]

	scenario(t, "validates iss field for equality", func(t *testing.T, sc scenarioContext) {
		tokenValid := sign(t, key, jwt.Claims{Issuer: "http://foo"})
		tokenInvalid := sign(t, key, jwt.Claims{Issuer: "http://bar"})

		_, err := sc.authJWTSvc.Verify(sc.ctx, tokenValid)
		require.NoError(t, err)

		_, err = sc.authJWTSvc.Verify(sc.ctx, tokenInvalid)
		require.Error(t, err)
	}, configurePKIXPublicKeyFile, func(t *testing.T, cfg *setting.Cfg) {
		cfg.JWTAuthExpectClaims = `{"iss": "http://foo"}`
	})

	scenario(t, "validates sub field for equality", func(t *testing.T, sc scenarioContext) {
		var err error

		tokenValid := sign(t, key, jwt.Claims{Subject: "foo"})
		tokenInvalid := sign(t, key, jwt.Claims{Subject: "bar"})

		_, err = sc.authJWTSvc.Verify(sc.ctx, tokenValid)
		require.NoError(t, err)

		_, err = sc.authJWTSvc.Verify(sc.ctx, tokenInvalid)
		require.Error(t, err)
	}, configurePKIXPublicKeyFile, func(t *testing.T, cfg *setting.Cfg) {
		cfg.JWTAuthExpectClaims = `{"sub": "foo"}`
	})

	scenario(t, "validates aud field for inclusion", func(t *testing.T, sc scenarioContext) {
		var err error

		_, err = sc.authJWTSvc.Verify(sc.ctx, sign(t, key, jwt.Claims{Audience: []string{"bar", "foo"}}))
		require.NoError(t, err)

		_, err = sc.authJWTSvc.Verify(sc.ctx, sign(t, key, jwt.Claims{Audience: []string{"foo", "bar", "baz"}}))
		require.NoError(t, err)

		_, err = sc.authJWTSvc.Verify(sc.ctx, sign(t, key, jwt.Claims{Audience: []string{"foo"}}))
		require.Error(t, err)

		_, err = sc.authJWTSvc.Verify(sc.ctx, sign(t, key, jwt.Claims{Audience: []string{"bar", "baz"}}))
		require.Error(t, err)

		_, err = sc.authJWTSvc.Verify(sc.ctx, sign(t, key, jwt.Claims{Audience: []string{"baz"}}))
		require.Error(t, err)
	}, configurePKIXPublicKeyFile, func(t *testing.T, cfg *setting.Cfg) {
		cfg.JWTAuthExpectClaims = `{"aud": ["foo", "bar"]}`
	})

	scenario(t, "validates non-registered (custom) claims for equality", func(t *testing.T, sc scenarioContext) {
		var err error

		_, err = sc.authJWTSvc.Verify(sc.ctx, sign(t, key, map[string]interface{}{"my-str": "foo", "my-number": 123}))
		require.NoError(t, err)

		_, err = sc.authJWTSvc.Verify(sc.ctx, sign(t, key, map[string]interface{}{"my-str": "bar", "my-number": 123}))
		require.Error(t, err)

		_, err = sc.authJWTSvc.Verify(sc.ctx, sign(t, key, map[string]interface{}{"my-str": "foo", "my-number": 100}))
		require.Error(t, err)

		_, err = sc.authJWTSvc.Verify(sc.ctx, sign(t, key, map[string]interface{}{"my-str": "foo"}))
		require.Error(t, err)

		_, err = sc.authJWTSvc.Verify(sc.ctx, sign(t, key, map[string]interface{}{"my-number": 123}))
		require.Error(t, err)
	}, configurePKIXPublicKeyFile, func(t *testing.T, cfg *setting.Cfg) {
		cfg.JWTAuthExpectClaims = `{"my-str": "foo", "my-number": 123}`
	})

	scenario(t, "validates exp claim of the token", func(t *testing.T, sc scenarioContext) {
		var err error

		// time.Now should be okay because of default one-minute leeway of go-jose library.
		_, err = sc.authJWTSvc.Verify(sc.ctx, sign(t, key, jwt.Claims{Expiry: jwt.NewNumericDate(time.Now())}))
		require.NoError(t, err)

		_, err = sc.authJWTSvc.Verify(sc.ctx, sign(t, key, jwt.Claims{Expiry: jwt.NewNumericDate(time.Now().Add(-time.Minute - time.Second))}))
		require.Error(t, err)
	}, configurePKIXPublicKeyFile)

	scenario(t, "validates nbf claim of the token", func(t *testing.T, sc scenarioContext) {
		var err error

		// time.Now should be okay because of default one-minute leeway of go-jose library.
		_, err = sc.authJWTSvc.Verify(sc.ctx, sign(t, key, jwt.Claims{NotBefore: jwt.NewNumericDate(time.Now())}))
		require.NoError(t, err)

		_, err = sc.authJWTSvc.Verify(sc.ctx, sign(t, key, jwt.Claims{NotBefore: jwt.NewNumericDate(time.Now().Add(time.Minute + time.Second))}))
		require.Error(t, err)
	}, configurePKIXPublicKeyFile)

	scenario(t, "validates iat claim of the token", func(t *testing.T, sc scenarioContext) {
		var err error

		// time.Now should be okay because of default one-minute leeway of go-jose library.
		_, err = sc.authJWTSvc.Verify(sc.ctx, sign(t, key, jwt.Claims{IssuedAt: jwt.NewNumericDate(time.Now())}))
		require.NoError(t, err)

		_, err = sc.authJWTSvc.Verify(sc.ctx, sign(t, key, jwt.Claims{IssuedAt: jwt.NewNumericDate(time.Now().Add(time.Minute + time.Second))}))
		require.Error(t, err)
	}, configurePKIXPublicKeyFile)
}

func jwkHTTPScenario(t *testing.T, desc string, fn scenarioFunc, cbs ...configureFunc) {
	t.Helper()
	t.Run(desc, func(t *testing.T) {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewEncoder(w).Encode(jwksPublic); err != nil {
				panic(err)
			}
		}))
		t.Cleanup(ts.Close)

		configure := func(t *testing.T, cfg *setting.Cfg) {
			cfg.JWTAuthJWKSetURL = ts.URL
		}
		runner := scenarioRunner(func(t *testing.T, sc scenarioContext) {
			keySet := sc.authJWTSvc.keySet.(*keySetHTTP)
			keySet.client = ts.Client()
			fn(t, sc)
		}, append([]configureFunc{configure}, cbs...)...)
		runner(t)
	})
}

func jwkCachingScenario(t *testing.T, desc string, fn cachingScenarioFunc, cbs ...configureFunc) {
	t.Helper()

	t.Run(desc, func(t *testing.T) {
		var reqCount int

		// We run a server that each call responds differently.
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if reqCount++; reqCount > 2 {
				panic("calling more than two times is not supported")
			}
			jwks := jose.JSONWebKeySet{
				Keys: []jose.JSONWebKey{jwksPublic.Keys[reqCount-1]},
			}
			if err := json.NewEncoder(w).Encode(jwks); err != nil {
				panic(err)
			}
		}))
		t.Cleanup(ts.Close)

		configure := func(t *testing.T, cfg *setting.Cfg) {
			cfg.JWTAuthJWKSetURL = ts.URL
			cfg.JWTAuthCacheTTL = time.Hour
		}
		runner := scenarioRunner(func(t *testing.T, sc scenarioContext) {
			keySet := sc.authJWTSvc.keySet.(*keySetHTTP)
			keySet.client = ts.Client()
			fn(t, cachingScenarioContext{scenarioContext: sc, reqCount: &reqCount})
		}, append([]configureFunc{configure}, cbs...)...)

		runner(t)
	})
}

func scenario(t *testing.T, desc string, fn scenarioFunc, cbs ...configureFunc) {
	t.Helper()

	t.Run(desc, scenarioRunner(fn, cbs...))
}

func initAuthService(t *testing.T, cbs ...configureFunc) (*AuthService, error) {
	sqlStore := sqlstore.InitTestDB(t)
	remoteCacheSvc := &remotecache.RemoteCache{}
	cfg := setting.NewCfg()
	cfg.JWTAuthEnabled = true
	cfg.JWTAuthExpectClaims = "{}"
	cfg.RemoteCacheOptions = &setting.RemoteCacheOptions{Name: "database"}
	for _, cb := range cbs {
		cb(t, cfg)
	}

	service := &AuthService{}

	err := registry.BuildServiceGraph([]interface{}{cfg}, []*registry.Descriptor{
		{
			Name:     sqlstore.ServiceName,
			Instance: sqlStore,
		},
		{
			Name:     remotecache.ServiceName,
			Instance: remoteCacheSvc,
		},
		{
			Name:     ServiceName,
			Instance: service,
		},
	})

	return service, err
}

func scenarioRunner(fn scenarioFunc, cbs ...configureFunc) func(t *testing.T) {
	return func(t *testing.T) {
		authJWTSvc, err := initAuthService(t, cbs...)
		require.NoError(t, err)

		fn(t, scenarioContext{
			ctx:        context.Background(),
			cfg:        authJWTSvc.Cfg,
			authJWTSvc: authJWTSvc,
		})
	}
}

func configurePKIXPublicKeyFile(t *testing.T, cfg *setting.Cfg) {
	t.Helper()

	file, err := ioutil.TempFile(os.TempDir(), "public-key-*.pem")
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := os.Remove(file.Name()); err != nil {
			panic(err)
		}
	})

	blockBytes, err := x509.MarshalPKIXPublicKey(rsaKeys[0].Public())
	require.NoError(t, err)

	require.NoError(t, pem.Encode(file, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: blockBytes,
	}))
	require.NoError(t, file.Close())

	cfg.JWTAuthKeyFile = file.Name()
}
