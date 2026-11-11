package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/AGlElectronics/rally/backend/internal/config"
	"github.com/AGlElectronics/rally/backend/internal/groupmatch"
)

// OIDCVerifier wraps the go-oidc verifier and exposes the subset of token
// claims Rally cares about. The verifier handles JWKS rotation via OIDC
// discovery; nothing identity-provider-specific lives in this code.
type OIDCVerifier struct {
	verifier *oidc.IDTokenVerifier
	issuer   string
	// groups reads the person's groups from the token (staff only); nil
	// means groups are ignored, which is how attendee tokens are handled.
	groups *groupmatch.Config
	// rejected are the normalised issuers of the other realm (see
	// RejectIssuers).
	rejected []string
}

// ErrOtherRealm is returned for a token issued by the other realm: an
// attendee token on the staff API, or a staff token on the public site.
var ErrOtherRealm = errors.New("token was issued by the other realm")

// Issuer is the issuer this verifier accepts, as published by the identity
// provider's discovery document.
func (v *OIDCVerifier) Issuer() string { return v.issuer }

// RejectIssuers makes Verify answer ErrOtherRealm, before any other check,
// for tokens whose iss is one of issuers (compared normalised). Staff and
// attendee verifiers name each other here at start-up.
func (v *OIDCVerifier) RejectIssuers(issuers ...string) {
	for _, iss := range issuers {
		if n := config.NormalizeIssuer(iss); n != "" {
			v.rejected = append(v.rejected, n)
		}
	}
}

// SeparateRealms checks at start-up that the staff and attendee verifiers
// discovered different issuers, and makes each refuse the other's tokens.
// Either may be nil (not configured).
func SeparateRealms(staff, attendee *OIDCVerifier) error {
	if staff == nil || attendee == nil {
		return nil
	}
	if config.NormalizeIssuer(staff.Issuer()) == config.NormalizeIssuer(attendee.Issuer()) {
		return fmt.Errorf("the attendee realm (%s) and the staff realm (%s) are the same issuer; attendees would get staff access. Point ATTENDEE_OIDC_ISSUER_URL at a separate realm",
			attendee.Issuer(), staff.Issuer())
	}
	staff.RejectIssuers(attendee.Issuer())
	attendee.RejectIssuers(staff.Issuer())
	return nil
}

// OIDCClaims is the parsed claim subset Rally stores.
type OIDCClaims struct {
	Issuer            string `json:"iss"`
	Subject           string `json:"sub"`
	Email             string `json:"email"`
	EmailVerified     bool   `json:"email_verified"`
	PreferredUsername string `json:"preferred_username"`
	Name              string `json:"name"`
	Nickname          string `json:"nickname"`
	// Groups come only from the claim named by OIDC_GROUPS_CLAIM (shared
	// groupmatch rule); the "roles" claim is never merged in. Always empty
	// for attendees.
	Groups []string `json:"-"`
}

// NewOIDCVerifier runs OIDC discovery and returns a verifier configured for
// Rally's staff audience. Call once at startup; the returned object is safe
// for concurrent use.
func NewOIDCVerifier(ctx context.Context, cfg *config.Config) (*OIDCVerifier, error) {
	if cfg.OIDCIssuerURL == "" || cfg.OIDCClientID == "" {
		return nil, errors.New("OIDC verifier requires OIDC_ISSUER_URL and OIDC_CLIENT_ID")
	}
	audience := cfg.OIDCAudience
	if audience == "" {
		audience = cfg.OIDCClientID
	}
	v, err := NewOIDCVerifierFor(ctx, cfg.OIDCIssuerURL, audience)
	if err != nil {
		return nil, err
	}
	groups := cfg.Groups
	v.groups = &groups
	return v, nil
}

// NewOIDCVerifierFor builds a verifier for one issuer and audience that
// ignores group claims. Staff and attendees each get their own, so a token
// from one realm never passes the other's checks.
func NewOIDCVerifierFor(ctx context.Context, issuer, audience string) (*OIDCVerifier, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	// go-oidc already refuses a discovery document for another issuer;
	// keep the issuer exactly as published.
	var meta struct {
		Issuer string `json:"issuer"`
	}
	if err := provider.Claims(&meta); err != nil || meta.Issuer == "" {
		return nil, fmt.Errorf("oidc discovery: no issuer in the discovery document of %s", issuer)
	}
	return &OIDCVerifier{
		verifier: provider.Verifier(&oidc.Config{ClientID: audience}),
		issuer:   meta.Issuer,
	}, nil
}

// unverifiedIssuer reads iss from a JWT without checking anything; only for
// telling realms apart before the real verification.
func unverifiedIssuer(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return ""
	}
	var c struct {
		Issuer string `json:"iss"`
	}
	if json.Unmarshal(payload, &c) != nil {
		return ""
	}
	return c.Issuer
}

// Verify parses and validates a bearer token. On success the issuer claim
// is populated from the verifier config so downstream code never has to
// trust a client-sent value.
func (v *OIDCVerifier) Verify(ctx context.Context, bearer string) (*OIDCClaims, error) {
	bearer = strings.TrimSpace(bearer)
	if bearer == "" {
		return nil, errors.New("missing bearer token")
	}
	if iss := config.NormalizeIssuer(unverifiedIssuer(bearer)); iss != "" {
		for _, other := range v.rejected {
			if iss == other {
				return nil, ErrOtherRealm
			}
		}
	}
	idToken, err := v.verifier.Verify(ctx, bearer)
	if err != nil {
		return nil, fmt.Errorf("verify token: %w", err)
	}
	var c OIDCClaims
	if err := idToken.Claims(&c); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}
	if c.Subject == "" {
		return nil, errors.New("token missing sub claim")
	}
	// Pin the issuer to what the verifier was configured with. Keycloak
	// token issuers include trailing /realm paths; always use what we
	// trust, not what the token asserts.
	c.Issuer = v.issuer
	if v.groups != nil {
		var raw map[string]any
		if err := idToken.Claims(&raw); err != nil {
			return nil, fmt.Errorf("parse claims: %w", err)
		}
		c.Groups = v.groups.Groups(raw)
	}
	return &c, nil
}
