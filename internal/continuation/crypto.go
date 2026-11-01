package continuation

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
)

const envelopeBytes = 92

func (k *keys) seal(r Record) ([]byte, error) {
	plaintext, e := encodeRecord(r)
	if e != nil {
		return nil, e
	}
	defer clear(plaintext)
	sub := k.derive("aes256-gcm")
	defer clear(sub[:])
	block, e := aes.NewCipher(sub[:])
	if e != nil {
		return nil, failure(Unavailable)
	}
	aead, e := cipher.NewGCM(block)
	if e != nil {
		return nil, failure(Unavailable)
	}
	header := make([]byte, envelopeBytes)
	copy(header, "aic-sea2")
	copy(header[8:16], k.id[:])
	copy(header[16:32], r.snapshot.bytes[:])
	copy(header[32:64], r.scope.bytes[:])
	binary.BigEndian.PutUint64(header[64:72], uint64(r.created))
	binary.BigEndian.PutUint64(header[72:80], uint64(r.expires))
	if _, e := rand.Read(header[80:92]); e != nil {
		return nil, failure(Unavailable)
	}
	sealed := aead.Seal(header, header[80:92], plaintext, header[:80])
	if len(sealed) > MaxRecordBytes {
		clear(sealed)
		return nil, failure(Limit)
	}
	return sealed, nil
}
func (k *keys) open(b []byte) (Record, error) {
	if len(b) < envelopeBytes+16 || len(b) > MaxRecordBytes || string(b[:8]) != "aic-sea2" || !hmac.Equal(b[8:16], k.id[:]) {
		return newRecord(recordPayload{}), failure(Corrupt)
	}
	sub := k.derive("aes256-gcm")
	defer clear(sub[:])
	block, e := aes.NewCipher(sub[:])
	if e != nil {
		return newRecord(recordPayload{}), failure(Corrupt)
	}
	aead, e := cipher.NewGCM(block)
	if e != nil {
		return newRecord(recordPayload{}), failure(Corrupt)
	}
	plaintext, e := aead.Open(nil, b[80:92], b[92:], b[:80])
	if e != nil {
		return newRecord(recordPayload{}), failure(Corrupt)
	}
	defer clear(plaintext)
	r, e := decodeRecord(plaintext)
	if e != nil {
		return newRecord(recordPayload{}), e
	}
	if !hmac.Equal(r.snapshot.bytes[:], b[16:32]) || !hmac.Equal(r.scope.bytes[:], b[32:64]) || r.created != int64(binary.BigEndian.Uint64(b[64:72])) || r.expires != int64(binary.BigEndian.Uint64(b[72:80])) {
		r.Clear()
		return newRecord(recordPayload{}), failure(Corrupt)
	}
	return r, nil
}
func bundle(k *keys, owner Commitment, highwater int64) []byte {
	b := make([]byte, 120)
	copy(b, "aic-key1")
	copy(b[8:16], k.id[:])
	copy(b[16:48], k.root[:])
	copy(b[48:80], owner.bytes[:])
	binary.BigEndian.PutUint64(b[80:88], uint64(highwater))
	sub := k.derive("bundle-integrity")
	h := hmac.New(sha256.New, sub[:])
	_, _ = h.Write(b[:88])
	copy(b[88:], h.Sum(nil))
	return b
}
func decodeBundle(b []byte) (*keys, Commitment, int64, error) {
	if len(b) != 120 || string(b[:8]) != "aic-key1" {
		return nil, newCommitment(commitmentPayload{}), 0, failure(Unavailable)
	}
	k := ownedPointer(newKeys(keysPayload{}))
	copy(k.id[:], b[8:16])
	copy(k.root[:], b[16:48])
	sub := k.derive("bundle-integrity")
	h := hmac.New(sha256.New, sub[:])
	_, _ = h.Write(b[:88])
	if !hmac.Equal(h.Sum(nil), b[88:]) {
		clear(k.root[:])
		return nil, newCommitment(commitmentPayload{}), 0, failure(Unavailable)
	}
	var owner Commitment = newCommitment(commitmentPayload{})
	copy(owner.bytes[:], b[48:80])
	floor := int64(binary.BigEndian.Uint64(b[80:88]))
	if floor < minMillis || floor > maxMillis {
		clear(k.root[:])
		return nil, newCommitment(commitmentPayload{}), 0, failure(ClockInvalid)
	}
	return k, owner, floor, nil
}
