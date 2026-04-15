package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"math/big"
	"time"

	"haruki-cloud/config"

	"golang.org/x/crypto/bcrypt"
)

// ================= Code & ID Generation =================

func generateVerificationCode(length int) string {
	digits := "0123456789"
	code := make([]byte, length)
	for i := range code {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		code[i] = digits[n.Int64()]
	}
	return string(code)
}

func generateBotID() (int, error) {
	// 生成 8 位数字 bot_id (10000000 - 99999999)
	n, err := rand.Int(rand.Reader, big.NewInt(90000000))
	if err != nil {
		return 0, err
	}
	return int(n.Int64() + 10000000), nil
}

func generateCredential() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// ================= Credential Hashing & Verification =================

// hashCredential 使用 bcrypt 哈希 credential
func hashCredential(credential string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(credential), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

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

// ================= Session TTL =================

// getSessionTTL 获取 session 有效期，限制在 1~30 天
func getSessionTTL() time.Duration {
	days := config.Cfg.HarukiBotDB.SessionTTLDays
	if days <= 0 {
		days = 7
	}
	if days > 30 {
		days = 30
	}
	return time.Duration(days) * 24 * time.Hour
}
