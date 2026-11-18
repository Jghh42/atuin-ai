package credentials

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
)

// ClientTokenPrefix marks the local Atuin-client bearer token. It authenticates
// the Atuin CLI to this loopback server and is unrelated to upstream OAuth.
const ClientTokenPrefix = "aia1_"

const clientTokenFile = "client-token"
const clientTokenBytes = 32

var ErrNoClientToken = errors.New("client token not created yet; start the server or run init once")

// ValidClientToken accepts only the exact generated spelling: the prefix and
// 64 lowercase hex digits. Header and TOML handling rely on this alphabet.
func ValidClientToken(s string) bool {
	if len(s) != len(ClientTokenPrefix)+2*clientTokenBytes || !strings.HasPrefix(s, ClientTokenPrefix) {
		return false
	}
	for _, c := range s[len(ClientTokenPrefix):] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func newClientToken() (string, error) {
	var b [clientTokenBytes]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", ErrStore
	}
	return ClientTokenPrefix + hex.EncodeToString(b[:]), nil
}
