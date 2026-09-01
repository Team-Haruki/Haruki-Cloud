package auth

import (
	"crypto/subtle"

	"golang.org/x/crypto/bcrypt"
)

// ================= Credential Verification =================

// verifyCredential 验证 credential。兼容处理：先尝试 bcrypt 验证，
// 如果 stored 不是 bcrypt 格式（旧的明文记录），回退到常量时间比较。
func verifyCredential(stored, provided string) bool {
	if isBcryptHash(stored) {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(provided)) == nil
	}
	// 明文回退（兼容旧记录）
	return len(stored) > 0 && len(provided) > 0 &&
		subtle.ConstantTimeCompare([]byte(stored), []byte(provided)) == 1
}

// isBcryptHash 检查字符串是否为 bcrypt 哈希格式
func isBcryptHash(s string) bool {
	return len(s) == 60 && s[0] == '$' && s[1] == '2'
}
