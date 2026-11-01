package continuation

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"io"
	"sort"
	"strconv"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/jsoncheck"
)

// CanonicalArguments never uses float64. Existing-prefix lexemes remain exact;
// protected output additionally enforces serializer-stable signed/u64 integers.
func CanonicalArguments(raw []byte, protected bool) ([]byte, error) {
	if len(raw) == 0 || len(raw) > maxArgumentsBytes || !jsoncheck.ValidUnicode(raw) {
		return nil, failure(ProjectionUnsupported)
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	work := 0
	token := func() (json.Token, error) {
		if work >= MaxTokens {
			return nil, failure(Limit)
		}
		work++
		return d.Token()
	}
	var value func(int) ([]byte, error)
	value = func(depth int) ([]byte, error) {
		if depth > 64 {
			return nil, failure(Limit)
		}
		t, e := token()
		if e != nil {
			return nil, failure(ProjectionUnsupported)
		}
		switch t := t.(type) {
		case json.Delim:
			switch t {
			case '{':
				members := map[string][]byte{}
				var keys []string
				for d.More() {
					kt, e := token()
					k, ok := kt.(string)
					if e != nil || !ok {
						return nil, failure(ProjectionUnsupported)
					}
					if _, ok := members[k]; ok {
						return nil, failure(ProjectionUnsupported)
					}
					v, e := value(depth + 1)
					if e != nil {
						return nil, e
					}
					members[k] = v
					keys = append(keys, k)
				}
				end, e := token()
				if e != nil || end != json.Delim('}') {
					return nil, failure(ProjectionUnsupported)
				}
				sort.Strings(keys)
				out := []byte{'{'}
				for i, k := range keys {
					if i > 0 {
						out = append(out, ',')
					}
					b, _ := json.Marshal(k)
					out = append(out, b...)
					out = append(out, ':')
					out = append(out, members[k]...)
				}
				return append(out, '}'), nil
			case '[':
				out := []byte{'['}
				for i := 0; d.More(); i++ {
					if i > 0 {
						out = append(out, ',')
					}
					v, e := value(depth + 1)
					if e != nil {
						return nil, e
					}
					out = append(out, v...)
				}
				end, e := token()
				if e != nil || end != json.Delim(']') {
					return nil, failure(ProjectionUnsupported)
				}
				return append(out, ']'), nil
			default:
				return nil, failure(ProjectionUnsupported)
			}
		case json.Number:
			n := string(t)
			if protected && !canonicalInteger(n) {
				return nil, failure(ProjectionUnsupported)
			}
			return []byte(n), nil
		case string:
			b, _ := json.Marshal(t)
			return b, nil
		case bool:
			if t {
				return []byte("true"), nil
			}
			return []byte("false"), nil
		case nil:
			return []byte("null"), nil
		default:
			return nil, failure(ProjectionUnsupported)
		}
	}
	out, e := value(0)
	if e != nil {
		return nil, e
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, failure(ProjectionUnsupported)
	}
	return out, nil
}
func canonicalInteger(s string) bool {
	if s == "0" {
		return true
	}
	start := 0
	if len(s) > 0 && s[0] == '-' {
		start = 1
	}
	if len(s) <= start || s[start] < '1' || s[start] > '9' {
		return false
	}
	for _, c := range s[start+1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	if start == 1 {
		_, e := strconv.ParseInt(s, 10, 64)
		return e == nil
	}
	_, e := strconv.ParseUint(s, 10, 64)
	return e == nil
}
func (k *keys) arguments(raw []byte, protected bool) (Commitment, error) {
	b, e := CanonicalArguments(raw, protected)
	if e != nil {
		return newCommitment(commitmentPayload{}), e
	}
	defer clear(b)
	return keyed(k.derive("binding"), "arguments", string(b)), nil
}

// Prefix is a message-aware grammar, including empty messages and blocks. Only
// the separate release matcher permits the pinned assistant segmentation rule.
func (k *keys) prefix(messages []atuin.Message) (Prefix, error) {
	if len(messages) > atuin.MaxMessages {
		return newPrefix(prefixPayload{}), failure(Limit)
	}
	sub := k.derive("binding")
	h := hmac.New(sha256.New, sub[:])
	field(h, "TranscriptStart/"+ProjectionVersion)
	budget, blocks := atuin.MaxBodyBytes, 0
	put := func(s string) {
		if budget < 0 {
			return
		}
		if len(s) > budget {
			budget = -1
			return
		}
		budget -= len(s)
		field(h, s)
	}
	for _, m := range messages {
		put("MessageStart")
		put(m.Role)
		if m.Role != "assistant" && m.Role != "user" {
			return newPrefix(prefixPayload{}), failure(BindingMismatch)
		}
		for _, b := range m.Content {
			blocks++
			if blocks > atuin.MaxBlocks {
				return newPrefix(prefixPayload{}), failure(Limit)
			}
			put("BlockStart")
			put(b.Type)
			switch b.Type {
			case "text":
				put(b.Text)
			case "tool_use":
				put(b.ID)
				put(b.Name)
				args, e := CanonicalArguments(b.Input, false)
				if e != nil {
					return newPrefix(prefixPayload{}), e
				}
				put(string(args))
				clear(args)
			case "tool_result":
				put(b.ToolUseID)
				put(b.Content)
				put(strconv.FormatBool(b.IsError))
				put(strconv.FormatBool(b.Remote))
				if b.ContentLength == nil {
					put("absent")
				} else {
					put("present")
					put(strconv.FormatInt(*b.ContentLength, 10))
				}
			default:
				return newPrefix(prefixPayload{}), failure(BindingMismatch)
			}
			put("BlockEnd")
			if budget < 0 {
				return newPrefix(prefixPayload{}), failure(Limit)
			}
		}
		put("MessageEnd")
	}
	if budget < 0 {
		return newPrefix(prefixPayload{}), failure(Limit)
	}
	field(h, "TranscriptEnd")
	var out Prefix = newPrefix(prefixPayload{})
	out.messages = uint32(len(messages))
	copy(out.digest.bytes[:], h.Sum(nil))
	return out, nil
}
func (k *keys) text(s string) (TextExpectation, error) {
	if len(s) > maxVisibleTextBytes {
		return newTextExpectation(textExpectationPayload{}), failure(ProjectionUnsupported)
	}
	sum := sha256.Sum256([]byte(s))
	return k.textDigest(uint32(len(s)), sum), nil
}
func (k *keys) textDigest(n uint32, sum [32]byte) TextExpectation {
	return newTextExpectation(textExpectationPayload{bytes: n, digest: keyed(k.derive("binding"), "client-text", uintField(uint64(n)), string(sum[:]))})
}

// policy commits ACTUAL rendered instructions/ordered tool definitions plus
// normalized renderer inputs. Volatile turn context and invocation are omitted.
func (k *keys) policy(r atuin.Request, rendered, definitions []byte) (Commitment, error) {
	if len(rendered) > 8*atuin.MaxBodyBytes || len(definitions) > atuin.MaxBodyBytes || len(r.Config.UserContexts) > atuin.MaxBlocks || len(r.Config.Skills) > atuin.MaxBlocks {
		return newCommitment(commitmentPayload{}), failure(Limit)
	}
	sub := k.derive("binding")
	h := hmac.New(sha256.New, sub[:])
	field(h, "policy/"+PolicyVersion)
	byteField(h, rendered)
	byteField(h, definitions)
	budget := atuin.MaxBodyBytes
	put := func(s string) {
		if budget < 0 {
			return
		}
		if len(s) > budget {
			budget = -1
			return
		}
		budget -= len(s)
		field(h, s)
	}
	optional := func(p *string) {
		if p == nil {
			put("absent")
		} else {
			put("present")
			put(*p)
		}
	}
	if len(r.Config.Capabilities) > 128 {
		return newCommitment(commitmentPayload{}), failure(Limit)
	}
	caps := make([]string, 0, len(r.Config.Capabilities))
	for name, on := range r.Config.Capabilities {
		if on {
			caps = append(caps, name)
		}
	}
	sort.Strings(caps)
	put(strconv.Itoa(len(caps)))
	for _, s := range caps {
		put(s)
	}
	put(r.Config.RunPreference)
	for _, s := range []*string{r.Context.OS, r.Context.Distro, r.Context.Shell, r.Context.PreferredLanguage} {
		optional(s)
	}
	put(strconv.Itoa(len(r.Config.UserContexts)))
	for _, c := range r.Config.UserContexts {
		put(c.Path)
		put(c.Data)
	}
	put(strconv.Itoa(len(r.Config.Skills)))
	for _, s := range r.Config.Skills {
		put(s.Name)
		put(s.Description)
	}
	optional(r.Config.SkillsOverflow)
	if budget < 0 {
		return newCommitment(commitmentPayload{}), failure(Limit)
	}
	var out Commitment = newCommitment(commitmentPayload{})
	copy(out.bytes[:], h.Sum(nil))
	return out, nil
}
func uintField(v uint64) string {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return string(b[:])
}
