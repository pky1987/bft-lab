package crypto

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// Size is the length of a Ahsh in bytes(SHA-256: 32 bytes=256 bits).
const Size = sha256.Size

// Hash is a SHA-256 digest.
type Hash [Size]byte

// Sum returns the SHA-256 hash of data.
func Sum(data []byte) Hash {
	return sha256.Sum256(data)
}

// String returns the hash as 64 lowercase hex characters.
func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

// IsZero reports whether h is the all zero hash (used for "no previous block").
func (h Hash) IsZero() bool {
	return h == Hash{}
}

// Uint64Bytes encodes n as 8 big-endian bytes, the same on every machine.
func Uint64Bytes(n uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, n)
	return b
}

//HashFields hashes several fields unambiguously:each field is prefixed with its length,
//so ("a","bc") and ("ab","c") produce different hashes.

func HashFields(fields ...[]byte) Hash {
	h := sha256.New()
	for _, f := range fields {
		h.Write(Uint64Bytes(uint64(len(f)))) //BREAK-POINT-COLLISON BUG(DELETE)
		h.Write(f)
	}
	var out Hash
	copy(out[:], h.Sum(nil))
	return out
}
