package crypto

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	plain := "110101199001011234"

	ct, err := EncryptAESGCM(key, plain)
	if err != nil {
		t.Fatalf("EncryptAESGCM: %v", err)
	}
	if ct == plain {
		t.Fatal("密文不应等于明文")
	}

	got, err := DecryptAESGCM(key, ct)
	if err != nil {
		t.Fatalf("DecryptAESGCM: %v", err)
	}
	if got != plain {
		t.Fatalf("往返解密不一致: got %q want %q", got, plain)
	}

	if _, err := EncryptAESGCM([]byte("short-key"), plain); err == nil {
		t.Fatal("非 32 字节密钥应报错")
	}
	if _, err := DecryptAESGCM(key, "!!!not-base64!!!"); err == nil {
		t.Fatal("非法密文应报错")
	}
}

func TestMaskIDNo(t *testing.T) {
	got := MaskIDNo("110101199001011234")
	want := "110***********1234"
	if got != want {
		t.Fatalf("MaskIDNo = %q, want %q", got, want)
	}
	if masked := MaskIDNo("123"); strings.Contains(masked, "123") {
		t.Fatalf("短证号不应泄露原文: %q", masked)
	}
}

func TestMaskName(t *testing.T) {
	got := MaskName("王老师")
	want := "王*师"
	if got != want {
		t.Fatalf("MaskName = %q, want %q", got, want)
	}
	if masked := MaskName("王磊"); masked != "王*" {
		t.Fatalf("两字名 = %q, want %q", masked, "王*")
	}
}

func TestSHA256Hex(t *testing.T) {
	// "abc" 的 SHA-256 标准值
	want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := SHA256Hex("abc"); got != want {
		t.Fatalf("SHA256Hex(abc) = %q, want %q", got, want)
	}
}
