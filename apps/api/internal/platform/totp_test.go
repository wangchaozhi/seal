package platform

import (
	"encoding/base32"
	"testing"
	"time"
)

func TestTOTPVerification(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	code := totpCode(key, now.Unix()/30)
	if !verifyTOTP(secret, code, now) {
		t.Fatal("valid code rejected")
	}
	if verifyTOTP(secret, "000000", now) {
		t.Fatal("invalid code accepted")
	}
}
