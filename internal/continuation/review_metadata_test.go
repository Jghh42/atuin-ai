package continuation

import (
	"bytes"
	"testing"
)

func TestReviewMetadataCodecSymmetryAndOldVersionReject(t *testing.T) {
	k := testKeys()
	for phase := uint8(0); phase <= 2; phase++ {
		for caller := uint8(0); caller <= 2; caller++ {
			for async := uint8(0); async <= 2; async++ {
				r := scalarRecord(t, k)
				for _, s := range r.segments {
					for _, i := range s.items {
						switch i.kind {
						case 1:
							i.phasePresence = phase
							if phase == 2 {
								i.phase = "commentary"
							}
							for _, p := range i.content {
								p.logprobsPresence = caller
							}
						case 2:
							i.callerPresence = caller
							i.asyncPresence = async
						}
					}
				}
				encoded, e := encodeRecord(r)
				if e != nil {
					t.Fatal("valid metadata did not encode")
				}
				decoded, e := decodeRecord(append([]byte(nil), encoded...))
				if e != nil {
					t.Fatal("valid metadata did not decode")
				}
				again, e := encodeRecord(decoded)
				if e != nil || !bytes.Equal(encoded, again) {
					t.Fatal("metadata encode/decode asymmetry")
				}
				old := append([]byte(nil), encoded...)
				copy(old[8:], "aic-record1")
				if _, e := decodeRecord(old); e == nil {
					t.Fatal("old record layout accepted")
				}
				sealed, e := k.seal(r)
				if e != nil {
					t.Fatal(e)
				}
				copy(sealed[:8], "aic-sea1")
				if _, e := k.open(sealed); e == nil {
					t.Fatal("old envelope version accepted")
				}
			}
		}
	}
}
func TestReviewMetadataCodecRejectsInvalidScalars(t *testing.T) {
	k := testKeys()
	for _, mutate := range []func(Item){func(i Item) { i.phasePresence = 3 }, func(i Item) { i.phasePresence = 2; i.phase = "unsupported" }, func(i Item) { i.statusPresence = 0; i.status = "" }, func(i Item) { i.content[0].logprobsPresence = 3 }} {
		r := scalarRecord(t, k)
		for _, i := range r.segments[0].items {
			if i.kind == 1 {
				mutate(i)
			}
		}
		if _, e := encodeRecord(r); e == nil {
			t.Fatal("invalid message metadata encoded")
		}
	}
}
