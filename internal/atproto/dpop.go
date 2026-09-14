package atproto

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

func dpopProof(key *ecdsa.PrivateKey, method, target, nonce, accessToken string, nowUnix int64) (string, error) {
	jti, err := randomToken(18)
	if err != nil {
		return "", err
	}
	header := map[string]any{
		"typ": "dpop+jwt",
		"alg": "ES256",
		"jwk": map[string]string{
			"kty": "EC", "crv": "P-256", "x": padded32(key.PublicKey.X), "y": padded32(key.PublicKey.Y),
		},
	}
	claims := map[string]any{
		"jti": jti,
		"htm": strings.ToUpper(method),
		"htu": target,
		"iat": nowUnix,
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	if accessToken != "" {
		digest := sha256.Sum256([]byte(accessToken))
		claims["ath"] = base64.RawURLEncoding.EncodeToString(digest[:])
	}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)
	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	digest := sha256.Sum256([]byte(unsigned))
	r, sigS, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign DPoP proof: %w", err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(rawSignature(r, sigS)), nil
}
