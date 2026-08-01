package platform

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

func verifyTOTP(secret, provided string, now time.Time) bool {
	provided = strings.TrimSpace(provided)
	if secret == "" || len(provided) != 6 {
		return false
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.ReplaceAll(secret, " ", "")))
	if err != nil || len(key) < 10 {
		return false
	}
	for offset := int64(-1); offset <= 1; offset++ {
		expected := totpCode(key, now.Unix()/30+offset)
		if subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1 {
			return true
		}
	}
	return false
}

func totpCode(key []byte, counter int64) string {
	message := make([]byte, 8)
	binary.BigEndian.PutUint64(message, uint64(counter))
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(message)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1_000_000)
}
