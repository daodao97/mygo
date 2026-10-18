package sharedstore

import (
	"crypto/rand"
	"encoding/hex"
)

func randomName() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
