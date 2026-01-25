package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func NewID(prefix string) string {
	stamp := time.Now().UTC().Format("20060102-150405")
	suffix := randomHex(4)
	if suffix == "" {
		suffix = "00000000"
	}
	return fmt.Sprintf("%s-%s-%s", prefix, stamp, suffix)
}

func randomHex(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}
