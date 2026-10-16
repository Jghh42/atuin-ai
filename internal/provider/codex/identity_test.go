package codex

import (
	"context"
	"net/http"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/credentials"
)

type rejectTransport struct{ t *testing.T }

func (r rejectTransport) RoundTrip(*http.Request) (*http.Response, error) {
	r.t.Error("inference rebound across catalog identity")
	return nil, ErrTransport
}
func TestDiscoveryIdentityCannotRebindDuringInferenceLookup(t *testing.T) {
	expected := credentials.Identity{Provider: "codex", Account: "account-old", Epoch: 4}
	for _, changed := range []credentials.Identity{{Provider: "codex", Account: "account-new", Epoch: 5}, {Provider: "codex", Account: "account-old", Epoch: 5}} {
		source := credentials.SourceFunc(func(context.Context) (credentials.Credential, error) {
			return credentials.Credential{Identity: changed, AccessToken: "synthetic"}, nil
		})
		client := RequestClient{Credentials: source, Transport: rejectTransport{t}}
		ctx := credentials.BindIdentity(context.Background(), expected)
		if _, err := client.Open(ctx, decodedRequest(t, `[{"role":"user","content":"hello"}]`)); err != ErrCredentials {
			t.Fatal("lifecycle mismatch accepted", err)
		}
	}
}
