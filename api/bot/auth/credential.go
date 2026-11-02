package auth

import (
	"golang.org/x/crypto/bcrypt"
)

// ================= Credential Verification =================

// verifyCredential 验证 credential。stored 必须是 bcrypt 哈希；
// 其他格式（包括旧的明文记录）一律拒绝。
func verifyCredential(stored, provided string) bool {
	if !isBcryptHash(stored) || len(provided) == 0 {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(stored), []byte(provided)) == nil
}

// isBcryptHash 检查字符串是否为 bcrypt 哈希格式
func isBcryptHash(s string) bool {
	return len(s) == 60 && s[0] == '$' && s[1] == '2'
}
