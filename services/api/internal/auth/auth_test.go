package auth

import "testing"

func TestNormalizePhone(t *testing.T) {
	valid, err := NormalizePhone(" +447700900123 ")
	if err != nil || valid != "+447700900123" {
		t.Fatalf("valid phone = %q, %v", valid, err)
	}
	for _, phone := range []string{"447700900123", "+0012345678", "+44 7700 900123", "+123"} {
		if _, err := NormalizePhone(phone); err == nil {
			t.Errorf("%q should be invalid", phone)
		}
	}
}

func TestHashOTPIncludesSalt(t *testing.T) {
	if string(hashOTP([]byte("one"), "123456")) == string(hashOTP([]byte("two"), "123456")) {
		t.Fatal("salt must change hash")
	}
}
