package core

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
)

// DefaultSpace is the space the Persona extension shell uses.
const DefaultSpace = "default"

var (
	spaceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	accountID = regexp.MustCompile(`^a[0-9a-f]{12}$`)
	uuidLike  = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)
)

// ValidSpace says whether name can name a space (it becomes a directory name: spaces/<space>).
func ValidSpace(name string) bool { return spaceName.MatchString(name) }

// NewAccountID is a fresh account id: "a" and 12 hex digits, as the Lark gateway's agent bridge pairs them.
func NewAccountID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "a" + hex.EncodeToString(b[:])
}

// ValidAccountID says whether id has the account id format.
func ValidAccountID(id string) bool { return accountID.MatchString(id) }

// SessionKey is agent:<provider>:<accountId>:<externalId>.
func SessionKey(provider, accountID, externalID string) string {
	return "agent:" + provider + ":" + accountID + ":" + externalID
}

// ParseSessionKey splits a session key; the external id may itself contain colons.
func ParseSessionKey(key string) (provider, accountID, externalID string, ok bool) {
	parts := strings.SplitN(key, ":", 4)
	if len(parts) != 4 || parts[0] != "agent" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		return "", "", "", false
	}
	return parts[1], parts[2], parts[3], true
}

// ValidClientID says whether id is a UUID, as the hub's client ids are.
func ValidClientID(id string) bool { return uuidLike.MatchString(id) }
