package store

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

// ulidAlphabet is Crockford's base32, minus the characters a human might
// confuse (I, L, O, U), per the ULID spec.
const ulidAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewID returns a fresh ULID: 26 Crockford-base32 characters whose first 10
// encode the Unix-millisecond timestamp (48 bits) and whose last 16 encode 80
// random bits. Ids are unique across processes and sort by creation time,
// which is why they are ULIDs rather than autoincrement integers
// (docs/DESIGN.md §2); a ULID also lets CreateTask generate the id before
// its transaction opens, so the caller gets it back without a re-query.
func NewID() string {
	entropy := make([]byte, 10)
	if _, err := rand.Read(entropy); err != nil {
		// crypto/rand failing is an unrecoverable environment failure, not a
		// condition this function can report its way out of.
		panic(fmt.Sprintf("store: crypto/rand: %v", err))
	}
	return ulidFrom(time.Now().UnixMilli(), entropy)
}

// ulidFrom encodes a 48-bit millisecond timestamp and 80 bits of entropy into
// a 26-character ULID. Split out from NewID so the encoding is testable with
// fixed inputs.
func ulidFrom(ms int64, entropy []byte) string {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(ms))
	copy(b[8:], entropy)
	return base32Crockford(b[:])
}

// base32Crockford encodes 128 bits as 26 base32 characters (26*5 = 130 bits,
// so the final character carries two bits of zero padding).
func base32Crockford(b []byte) string {
	var out [26]byte
	var buffer uint64
	bits := 0
	n := 0
	for _, c := range b {
		buffer = (buffer << 8) | uint64(c)
		bits += 8
		for bits >= 5 {
			bits -= 5
			out[n] = ulidAlphabet[(buffer>>bits)&0x1f]
			n++
		}
	}
	if bits > 0 {
		out[n] = ulidAlphabet[(buffer<<(5-bits))&0x1f]
		n++
	}
	if n != len(out) {
		panic(fmt.Sprintf("store: internal: base32Crockford produced %d chars, want %d", n, len(out)))
	}
	return string(out[:])
}
