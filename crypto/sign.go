package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
)

// Key and signature sizes for Ed25519.
const (
	PublicKeySize = ed25519.PublicKeySize //32 bytes
	SignatureSize = ed25519.SignatureSize //64 bytes
	SeedSize      = ed25519.SeedSize      //32 bytes
	AddressSize   = 20
)

// ErrBadSeed is returned when a seed is not exactly SeedSie bytes.
var ErrBadSeed = errors.New("Seed must be 32 bytes")

// PublicKey identifies a validator. Anyone may see it.
type PublicKey [PublicKeySize]byte

// Signature proves that the holder of  aprivate key signed a message.
type Signature [SignatureSize]byte

// Address is a short identifier derived from a public key.
type Address [AddressSize]byte

// PrivateKey signs message. It must never be chared or logged.
type PrivateKey struct {
	key ed25519.PrivateKey //unexported:callers can sign , but an't read the secret.
}

// GenerateKey creates a new random key pair(for real nodes).
func GenerateKey() (PrivateKey, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return PrivateKey{}, err
	}
	return PrivateKey{key: priv}, nil
}

//NewKeyFromSeed erives a key pair from 32 bytes seed .The same seed always gives the same key.
//so, simulations are reproducible.

func NewKeyFromSeed(seed []byte) (PrivateKey, error) {
	if len(seed) != SeedSize {
		return PrivateKey{}, ErrBadSeed
	}
	return PrivateKey{key: ed25519.NewKeyFromSeed(seed)}, nil
}

// Public returns the publiic key that matches k.
func (k PrivateKey) Public() PublicKey {
	var pub PublicKey
	copy(pub[:], k.key.Public().(ed25519.PublicKey))
	return pub
}

// Sign sign msg. Ed25519 signatures are deterministic:same key+msg=same signatures.
func (k PrivateKey) Sign(msg []byte) Signature {
	var sig Signature
	copy(sig[:], ed25519.Sign(k.key, msg))
	return sig
}

// Verify reports whether sig is  avlid signature of msg by pub.
func Verify(pub PublicKey, msg []byte, sig Signature) bool {
	return ed25519.Verify(pub[:], msg, sig[:])
}

// Address returns the first 20 bytes of SHA-256(pub),like CometBFT validator addresses.
func (p PublicKey) Address() Address {
	h := Sum(p[:])
	var a Address
	copy(a[:], h[:AddressSize])
	return a
}

//String returns the address as 40 hex characters.

func (a Address) String() string {
	return hex.EncodeToString(a[:])
}
