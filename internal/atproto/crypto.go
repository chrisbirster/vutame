package atproto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
)

func (s *Store) encryptionKey() []byte {
	hash := sha256.New()
	_, _ = hash.Write([]byte("vutame-atproto-encryption-v1\x00"))
	_, _ = hash.Write(s.secret)
	return hash.Sum(nil)
}

func (s *Store) encrypt(value []byte) (string, error) {
	block, err := aes.NewCipher(s.encryptionKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, value, []byte("vutame-atproto-v1"))
	payload := append(nonce, sealed...)
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func (s *Store) decrypt(encoded string) ([]byte, error) {
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("decode encrypted ATProto credential")
	}
	block, err := aes.NewCipher(s.encryptionKey())
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(payload) < gcm.NonceSize() {
		return nil, errors.New("invalid encrypted ATProto credential")
	}
	plain, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], []byte("vutame-atproto-v1"))
	if err != nil {
		return nil, errors.New("decrypt ATProto credential")
	}
	return plain, nil
}

func generateDPoPKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

func marshalDPoPKey(key *ecdsa.PrivateKey) ([]byte, error) {
	return x509.MarshalPKCS8PrivateKey(key)
}

func parseDPoPKey(value []byte) (*ecdsa.PrivateKey, error) {
	parsed, err := x509.ParsePKCS8PrivateKey(value)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errors.New("invalid ATProto DPoP key")
	}
	return key, nil
}

func randomToken(bytesCount int) (string, error) {
	value := make([]byte, bytesCount)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func pkceChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func stateHash(state string) string {
	digest := sha256.Sum256([]byte(state))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func padded32(value *big.Int) string {
	bytes := value.Bytes()
	buffer := make([]byte, 32)
	copy(buffer[len(buffer)-len(bytes):], bytes)
	return base64.RawURLEncoding.EncodeToString(buffer)
}

func rawSignature(r, s *big.Int) []byte {
	result := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(result[32-len(rBytes):32], rBytes)
	copy(result[64-len(sBytes):], sBytes)
	return result
}

// Keep crypto/ecdh referenced so Go's standard P-256 implementation remains
// selected by the runtime even when elliptic internals evolve.
var _ = ecdh.P256
var _ = fmt.Sprintf
