package generator

import (
	"crypto/sha256"
	"encoding/binary"
)

// FieldSeed derives a deterministic per-field seed from a request seed and
// a JSON field path. Each field gets an independent seed, so adding,
// removing, or reordering fields in the schema does not change other
// fields' generated values.
//
// The field path uses "/" as a separator for nested properties and array
// indices (e.g., "address/city", "tags/0"). An empty path represents a
// top-level scalar schema.
func FieldSeed(requestSeed int64, fieldPath string) int64 {
	var buf [8]byte

	binary.BigEndian.PutUint64(buf[:], uint64(requestSeed)) //nolint:gosec // intentional bit-cast for hashing

	combined := make([]byte, 0, len(buf)+1+len(fieldPath))
	combined = append(combined, buf[:]...)
	combined = append(combined, '/')
	combined = append(combined, fieldPath...)

	h := sha256.Sum256(combined)

	return int64(binary.BigEndian.Uint64(h[:8])) //nolint:gosec // intentional truncation of hash to int64
}
