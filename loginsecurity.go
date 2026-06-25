package main

import (
	"crypto/subtle"
	"strings"
	"sync"
	"time"
)

const (
	minPasswordLen      = 8
	maxLoginFails       = 5                // lock after this many consecutive failures
	loginLockDuration   = 10 * time.Minute // lockout window after too many failures
	loginFailWindow     = 10 * time.Minute // counting window for failures
	loginRecordsMaxKeep = 10000            // cap memory usage
)

// weakPasswords is a small blocklist of common weak passwords. Registration
// rejects any password whose lowercase form hits this list.
var weakPasswords = map[string]bool{
	"12345678": true, "123456789": true, "1234567890": true,
	"password": true, "password1": true, "passw0rd": true,
	"qwertyui": true, "11111111": true, "00000000": true,
	"abcd1234": true, "admin123": true, "iloveyou": true,
	"87654321": true, "1qaz2wsx": true, "qazwsxedc": true,
	"openp2p123": true, "administrator": true,
}

// checkPasswordStrength validates that a password is not weak.
// Rules: length>=8, not in blocklist, and must contain at least two of
// {lowercase, uppercase, digit, symbol} character classes.
func checkPasswordStrength(pwd string) (bool, string) {
	if len(pwd) < minPasswordLen {
		return false, "密码至少需要8个字符"
	}
	if weakPasswords[strings.ToLower(pwd)] {
		return false, "密码过于简单，请勿使用常见弱口令"
	}
	var hasLower, hasUpper, hasDigit, hasSymbol bool
	for _, r := range pwd {
		switch {
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			hasSymbol = true
		}
	}
	classes := 0
	for _, ok := range []bool{hasLower, hasUpper, hasDigit, hasSymbol} {
		if ok {
			classes++
		}
	}
	if classes < 2 {
		return false, "密码需包含大小写字母、数字、符号中至少两类"
	}
	return true, ""
}

// constantTimeEqual compares two strings without leaking timing information.
func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// loginRecord tracks failed login attempts for one key(IP or user).
type loginRecord struct {
	fails      int
	firstFail  time.Time
	lockedTill time.Time
}

// loginGuard provides brute-force protection by throttling failed logins.
type loginGuard struct {
	mtx     sync.Mutex
	records map[string]*loginRecord
}

var gLoginGuard = &loginGuard{records: make(map[string]*loginRecord)}

// allowed reports whether the key is currently allowed to attempt a login.
// When locked, it returns false and the remaining lock seconds.
func (g *loginGuard) allowed(key string) (bool, int) {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	r := g.records[key]
	if r == nil {
		return true, 0
	}
	now := time.Now()
	if now.Before(r.lockedTill) {
		return false, int(time.Until(r.lockedTill).Seconds()) + 1
	}
	// reset stale counters outside the counting window
	if r.fails > 0 && now.Sub(r.firstFail) > loginFailWindow {
		r.fails = 0
	}
	return true, 0
}

// onFailure records a failed login and locks the key if threshold exceeded.
func (g *loginGuard) onFailure(key string) {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	now := time.Now()
	r := g.records[key]
	if r == nil {
		if len(g.records) > loginRecordsMaxKeep {
			g.gcLocked(now)
		}
		r = &loginRecord{firstFail: now}
		g.records[key] = r
	}
	if now.Sub(r.firstFail) > loginFailWindow {
		r.fails = 0
		r.firstFail = now
	}
	r.fails++
	if r.fails >= maxLoginFails {
		r.lockedTill = now.Add(loginLockDuration)
		r.fails = 0
		r.firstFail = now
	}
}

// onSuccess clears the failure record for the key.
func (g *loginGuard) onSuccess(key string) {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	delete(g.records, key)
}

// gcLocked removes expired records. Caller must hold the lock.
func (g *loginGuard) gcLocked(now time.Time) {
	for k, r := range g.records {
		if now.After(r.lockedTill) && now.Sub(r.firstFail) > loginFailWindow {
			delete(g.records, k)
		}
	}
}
