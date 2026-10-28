// Package crypto 提供敏感字段三件套工具：Enc（AES-256-GCM）、Hash（SHA-256）、Mask（脱敏）。
// 日志与接口只允许输出 Mask 结果，Enc/Hash 仅用于落库。
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

const aesKeyLen = 32

// EncryptAESGCM 使用 AES-256-GCM 加密明文，返回 base64(nonce+ciphertext)。
// key 必须为 32 字节。
func EncryptAESGCM(key []byte, plain string) (string, error) {
	if len(key) != aesKeyLen {
		return "", errors.New("crypto: key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptAESGCM 解密 EncryptAESGCM 的输出，还原明文。
func DecryptAESGCM(key []byte, ct string) (string, error) {
	if len(key) != aesKeyLen {
		return "", errors.New("crypto: key must be 32 bytes")
	}
	raw, err := base64.StdEncoding.DecodeString(ct)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("crypto: ciphertext too short")
	}
	nonce, sealed := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// SHA256Hex 返回字符串的 SHA-256 十六进制摘要（用于等值比对，不可逆）。
func SHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// MaskIDNo 脱敏证件号：保留前 3 后 4，中间以 * 代替；
// 长度不足 10 时全遮蔽（前3后4可见字符占比过高，防泄露）。
func MaskIDNo(idNo string) string {
	if len(idNo) < 10 {
		return strings.Repeat("*", len(idNo))
	}
	return idNo[:3] + strings.Repeat("*", len(idNo)-7) + idNo[len(idNo)-4:]
}

// MaskName 脱敏姓名：保留首尾各 1 字，中间以 * 代替；两字名保留首字；单字名全遮蔽。
func MaskName(name string) string {
	runes := []rune(name)
	switch len(runes) {
	case 0:
		return ""
	case 1:
		return "*"
	case 2:
		return string(runes[0]) + "*"
	default:
		return string(runes[0]) + strings.Repeat("*", len(runes)-2) + string(runes[len(runes)-1])
	}
}
