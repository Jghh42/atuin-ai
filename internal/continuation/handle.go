package continuation

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"strings"
)

const HandlePrefix = "aic1_"
const HandleBytes = 90

type keysPayload struct {
	root [32]byte
	id   [8]byte
}

func (k *keys) derive(domain string) [32]byte {
	h := hmac.New(sha256.New, k.root[:])
	field(h, "atuin-ai/continuation-v1/"+domain)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}
func byteField(h hash.Hash, b []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(b)))
	_, _ = h.Write(length[:])
	_, _ = h.Write(b)
}
func field(h hash.Hash, s string) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(len(s)))
	_, _ = h.Write(b[:])
	_, _ = h.Write([]byte(s))
}
func keyed(key [32]byte, domain string, values ...string) Commitment {
	h := hmac.New(sha256.New, key[:])
	field(h, domain)
	for _, s := range values {
		field(h, s)
	}
	var c Commitment = newCommitment(commitmentPayload{})
	copy(c.bytes[:], h.Sum(nil))
	return c
}
func equal(a, b Commitment) bool {
	var av, bv [32]byte
	if a.commitmentPayload != nil {
		av = a.bytes
	}
	if b.commitmentPayload != nil {
		bv = b.bytes
	}
	return hmac.Equal(av[:], bv[:])
}
func Reserved(s string) bool { return strings.HasPrefix(s, HandlePrefix) }
func validHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

type handlePartsPayload struct {
	key      [8]byte
	snapshot SnapshotID
	ordinal  uint8
	mac      [16]byte
}

func parseHandle(s string) (handleParts, error) {
	var p handleParts = newHandleParts(handlePartsPayload{})
	if len(s) != HandleBytes || !Reserved(s) || s[21] != '_' || s[54] != '_' || s[57] != '_' || !validHex(s[5:21], 16) || !validHex(s[22:54], 32) || !validHex(s[55:57], 2) || !validHex(s[58:], 32) {
		return p, failure(HandleInvalid)
	}
	_, _ = hex.Decode(p.key[:], []byte(s[5:21]))
	_, _ = hex.Decode(p.snapshot.bytes[:], []byte(s[22:54]))
	var o [1]byte
	_, _ = hex.Decode(o[:], []byte(s[55:57]))
	p.ordinal = o[0]
	_, _ = hex.Decode(p.mac[:], []byte(s[58:]))
	if p.ordinal >= MaxCalls {
		return newHandleParts(handlePartsPayload{}), failure(HandleInvalid)
	}
	return p, nil
}
func (k *keys) handle(snapshot SnapshotID, ordinal uint8, scope Commitment) (PublicHandle, error) {
	if ordinal >= MaxCalls {
		return newPublicHandle(publicHandlePayload{}), failure(Limit)
	}
	sub := k.derive("handles")
	mac := keyed(sub, "aic1", string(k.id[:]), string(snapshot.bytes[:]), string([]byte{ordinal}), string(scope.bytes[:]))
	return newPublicHandle(publicHandlePayload{value: HandlePrefix + hex.EncodeToString(k.id[:]) + "_" + hex.EncodeToString(snapshot.bytes[:]) + "_" + hex.EncodeToString([]byte{ordinal}) + "_" + hex.EncodeToString(mac.bytes[:16])}), nil
}
func (k *keys) verify(h PublicHandle, scope Commitment) (handleParts, error) {
	p, e := parseHandle(h.value)
	if e != nil {
		return p, e
	}
	expected, e := k.handle(p.snapshot, p.ordinal, scope)
	if e != nil || !hmac.Equal([]byte(h.value), []byte(expected.value)) {
		return newHandleParts(handlePartsPayload{}), failure(BindingMismatch)
	}
	return p, nil
}
func (k *keys) scope(in ScopeInput) Commitment {
	sub := k.derive("binding")
	var epoch [8]byte
	binary.BigEndian.PutUint64(epoch[:], in.identity.Epoch)
	return keyed(sub, "scope", in.identity.Provider, in.identity.Account, string(epoch[:]), in.model, in.session, ProjectionVersion, string(in.policy.bytes[:]))
}
func (k *keys) owner(idProvider, idAccount string, epoch uint64) Commitment {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], epoch)
	return keyed(k.derive("ownership"), "owner", idProvider, idAccount, string(b[:]))
}
