package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Passwords are hashed with Argon2id and stored in the PHC string format,
// $argon2id$v=19$m=19456,t=2,p=1$<salt>$<key>. The parameters travel with
// each hash, so they can be raised later without breaking existing passwords.

// OWASP's recommended Argon2id minimum: 19 MiB, 2 passes, 1 thread. Light
// enough for a small VPS.
const (
	argonMemory  = 19 * 1024 // KiB
	argonPasses  = 2
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
)

// minPasswordLength is the only password rule (PRD §8). It counts characters,
// not bytes, so a Cyrillic password isn't held to half the length.
const minPasswordLength = 6

func hashPassword(password string) string {
	salt := make([]byte, argonSaltLen)
	rand.Read(salt) // never returns an error since Go 1.24
	key := argon2.IDKey([]byte(password), salt, argonPasses, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonPasses, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

// checkPassword reports whether password matches hash. The error is for a
// hash that can't be parsed, never for a wrong password.
func checkPassword(hash, password string) (bool, error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false, errors.New("password hash is not in argon2id PHC format")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, fmt.Errorf("unsupported argon2 version %q", parts[2])
	}
	var memory, passes uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &passes, &threads); err != nil {
		return false, fmt.Errorf("parse argon2 parameters %q: %w", parts[3], err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decode argon2 salt: %w", err)
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("decode argon2 key: %w", err)
	}
	other := argon2.IDKey([]byte(password), salt, passes, memory, threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(key, other) == 1, nil
}
