package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// ---- 密码哈希（纯标准库 PBKDF2-HMAC-SHA256，100000 次迭代，随机盐） ----
// 存储格式：sha256$<iter>$<saltHex>$<dkHex>，避免明文存放管理员密码。

const passwordIterations = 100000

func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen
	var buf [4]byte
	dk := make([]byte, 0, numBlocks*hashLen)
	u := make([]byte, hashLen)
	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(buf[:], uint32(block))
		prf.Write(buf[:])
		t := prf.Sum(nil)
		copy(u, t)
		dk = append(dk, u...)
		for n := 2; n <= iter; n++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for x := range t {
				t[x] ^= u[x]
			}
		}
	}
	return dk[:keyLen]
}

// hashPassword 生成 sha256$iter$salt$hash 格式的密码哈希
func hashPassword(pw string) (string, error) {
	if pw == "" {
		return "", fmt.Errorf("密码不能为空")
	}
	salt := make([]byte, 12)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk := pbkdf2SHA256([]byte(pw), salt, passwordIterations, 32)
	return fmt.Sprintf("sha256$%d$%s$%x", passwordIterations, hex.EncodeToString(salt), dk), nil
}

// checkPassword 校验密码是否匹配存储的哈希
func checkPassword(pw, stored string) bool {
	if pw == "" || stored == "" {
		return false
	}
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 || iter > 10_000_000 {
		return false
	}
	salt, err := hex.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got := pbkdf2SHA256([]byte(pw), salt, iter, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// isPasswordHashed 判断配置里的密码是否已是哈希格式（用于迁移）
func isPasswordHashed(v string) bool {
	return strings.HasPrefix(v, "sha256$")
}

// isUnsafeInline 判断扩展名是否属于可能携带脚本、应强制附件下载（禁止内联预览）的类型
func isUnsafeInline(ext string) bool {
	switch strings.ToLower(ext) {
	case "html", "htm", "shtml", "xhtml",
		"svg", "svgz", "xml",
		"js", "mjs", "css":
		return true
	}
	return false
}
