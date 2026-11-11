package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeIssuer serves OIDC discovery and signing keys for any number of realms
// under /realms/<name>, all signed with one RSA key.
type fakeIssuer struct {
	t   *testing.T
	srv *httptest.Server
	key *rsa.PrivateKey
}

func newFakeIssuer(t *testing.T) *fakeIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIssuer{t: t, key: key}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

// issuer is the issuer URL of realm.
func (f *fakeIssuer) issuer(realm string) string { return f.srv.URL + "/realms/" + realm }

func (f *fakeIssuer) serve(w http.ResponseWriter, r *http.Request) {
	realm, rest, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/realms/"), "/")
	w.Header().Set("Content-Type", "application/json")
	switch rest {
	case ".well-known/openid-configuration":
		iss := f.issuer(realm)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                iss,
			"authorization_endpoint":                iss + "/auth",
			"token_endpoint":                        iss + "/token",
			"jwks_uri":                              iss + "/certs",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	case "certs":
		b64 := base64.RawURLEncoding.EncodeToString
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": "k1", "use": "sig", "alg": "RS256",
			"n": b64(f.key.N.Bytes()), "e": b64(big.NewInt(int64(f.key.E)).Bytes()),
		}}})
	default:
		http.NotFound(w, r)
	}
}

// token signs claims; iss, exp and iat get defaults when not set.
func (f *fakeIssuer) token(realm string, claims map[string]any) string {
	f.t.Helper()
	c := map[string]any{"iss": f.issuer(realm), "exp": time.Now().Add(5 * time.Minute).Unix(), "iat": time.Now().Unix()}
	for k, v := range claims {
		c[k] = v
	}
	enc := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			f.t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	signing := enc(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"}) + "." + enc(c)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, sum[:])
	if err != nil {
		f.t.Fatal(err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}
