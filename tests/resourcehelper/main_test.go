package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/application"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

func TestObservedTCPPartialTimeoutWitness(t *testing.T) {
 base,e:=net.Listen("tcp4","127.0.0.1:0");if e!=nil{t.Fatal(e)}
 listener:=&observedListener{Listener:base};defer listener.Close()
 client,e:=net.DialTimeout("tcp4",base.Addr().String(),time.Second);if e!=nil{t.Fatal(e)};defer client.Close()
 if e:=client.(*net.TCPConn).SetReadBuffer(1024);e!=nil{t.Fatal(e)}
 server,e:=listener.Accept();if e!=nil{t.Fatal(e)}
 if e:=server.SetWriteDeadline(time.Now().Add(30*time.Millisecond));e!=nil{t.Fatal(e)}
 n,e:=server.Write(make([]byte,4<<20))
 if failure,ok:=e.(net.Error);!ok || !failure.Timeout(){t.Fatal("not an actual TCP timeout")}
 stats:=listener.stats();w:=stats["write_timeout"].(*writeWitness)
 if n>=4<<20 || w.Written!=n || w.EndNS<w.DeadlineNS || w.StartNS>=w.DeadlineNS {t.Fatal("invalid partial timeout witness")}
 if e:=server.Close();e!=nil{t.Fatal(e)}
 if listener.stats()["active_tcp"].(int)!=0{t.Fatal("observed connection not released")}
}

func TestSetupTransportDenyDefault(t *testing.T) {
	for _, destination := range []string{"http://auth.openai.com/api/accounts/deviceauth/usercode", "https://else.invalid/api/accounts/deviceauth/usercode", "https://auth.openai.com/other", "https://auth.openai.com/api/accounts/deviceauth/usercode?query=1"} {
		transport := &setupTransport{}
		r, _ := http.NewRequest("POST", destination, strings.NewReader(`{"client_id":"`+credentials.ClientID+`"}`))
		if _, e := transport.RoundTrip(r); e == nil {
			t.Fatal("unexpected setup destination accepted")
		}
	}
}
func TestFreshLoginClosesAndReacquiresStore(t *testing.T) {
	root := t.TempDir()
	if e := os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(root, "credentials")
	claims, _ := json.Marshal(map[string]any{"exp": time.Now().Add(2 * time.Hour).Unix(), "https://api.openai.com/auth": map[string]string{"chatgpt_account_id": "synthetic-account"}})
	fake := synthetic{Token: "e30." + base64.RawURLEncoding.EncodeToString(claims) + ".c2ln", Account: "synthetic-account"}
	transport := &setupTransport{fake: fake}
	app, e := application.NewWithOptions(path, config.Default(), application.Options{Transport: transport})
	if e != nil {
		t.Fatal(e)
	}
	if e := app.Manager.Login(context.Background(), func(credentials.DevicePrompt) error { return nil }); e != nil {
		t.Fatal("fake reviewed login failed")
	}
	if transport.step != 3 {
		t.Fatal("wrong auth call count")
	}
	if e := app.Close(); e != nil {
		t.Fatal(e)
	}
	second, e := application.NewWithOptions(path, config.Default(), application.Options{Transport: &setupTransport{}})
	if e != nil {
		t.Fatal("setup retained lock")
	}
	defer second.Close()
	current, e := second.Manager.Current(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if current.Identity.Account != fake.Account || current.AccessToken != fake.Token {
		t.Fatal("synthetic binding mismatch")
	}
	entries, e := os.ReadDir(path)
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		info, e := entry.Info()
		if e != nil {
			t.Fatal(e)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatal("unsafe synthetic store mode")
		}
	}
}
func TestSetupExchangeRejectsExtraFormFields(t *testing.T) {
	v := url.Values{"grant_type": {"authorization_code"}, "client_id": {credentials.ClientID}, "code": {"resource-code"}, "code_verifier": {strings.Repeat("v", 43)}, "redirect_uri": {"https://auth.openai.com/deviceauth/callback"}, "extra": {"forbidden"}}
	r, _ := http.NewRequest("POST", "https://auth.openai.com/oauth/token", strings.NewReader(v.Encode()))
	transport := &setupTransport{step: 2}
	if _, e := transport.RoundTrip(r); e == nil {
		t.Fatal("extra field accepted")
	}
	r.Body = io.NopCloser(strings.NewReader("broken%"))
	if _, e := transport.RoundTrip(r); e == nil {
		t.Fatal("malformed form accepted")
	}
}
