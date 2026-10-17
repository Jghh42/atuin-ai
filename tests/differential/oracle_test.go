package differential

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/tests/compatfixture"
	"github.com/Jghh42/atuin-ai/tests/testutil"
)

const alpine = "alpine@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce"

func docker(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/docker", append([]string{"--host=unix:///var/run/docker.sock"}, args...)...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("docker %s: %v\n%s", args[0], e, out)
	}
	return strings.TrimSpace(string(out))
}
func fileSHA(t *testing.T, path string) string {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func oracleRequest(t *testing.T, id, endpoint string, body []byte) (int, string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/docker", "--host=unix:///var/run/docker.sock", "exec", "-i", id, "/fakecc", "request", endpoint)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
	cmd.Stdin = strings.NewReader(string(body))
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("oracle-only HTTP helper: %v %s", e, out)
	}
	var result struct {
		Status  int    `json:"status"`
		Body    string `json:"body"`
		Session string `json:"session"`
	}
	if json.Unmarshal(out, &result) != nil {
		t.Fatal("invalid oracle helper envelope")
	}
	return result.Status, result.Body, result.Session
}
func get(t *testing.T, base, path string) []byte {
	t.Helper()
	c := http.Client{Timeout: 4 * time.Second}
	r, e := c.Get(base + path)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	b, e := io.ReadAll(r.Body)
	if e != nil || r.StatusCode != 200 {
		t.Fatalf("GET status=%d error=%v", r.StatusCode, e)
	}
	return b
}
func TestOriginalReferenceOracle(t *testing.T) {
	path := os.Getenv("ATUIN_ORACLE_MANIFEST")
	if path == "" {
		t.Skip("BLOCKED pinned-source differential gate: build-oracle.py fresh manifest required")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var m struct {
		Server    string `json:"server_sha"`
		Core      string `json:"core_sha"`
		Image     string `json:"image_id"`
		Archive   string `json:"source_archive_sha256"`
		Recipe    string `json:"dockerfile_sha256"`
		Builder   string `json:"builder_sha256"`
		Support   string `json:"support_sha256"`
		Fresh     bool   `json:"fresh_inputs"`
		Unchanged bool   `json:"source_worktree_unchanged"`
	}
	if json.Unmarshal(b, &m) != nil || m.Server != "4d582bc5ceea5b5edfdcf3abb49dc850400cda7c" || m.Core != "565877e97ed3ff4840481b8d47e12eb2f9854039" || !strings.HasPrefix(m.Image, "sha256:") {
		t.Fatal("oracle source/image manifest mismatch")
	}
	if !m.Fresh || !m.Unchanged || m.Archive != "c9424d674e1a237b7f3332f95e7fd26a048e27d72027f0e8f3b87f8464382b95" || m.Recipe != fileSHA(t, "Dockerfile.oracle") || m.Builder != fileSHA(t, "build-oracle.py") || m.Support != fileSHA(t, "../build_support.py") {
		t.Fatal("stale/inconsistent build recipe/archive proof; rebuild pinned oracle")
	}
	labels := docker(t, "image", "inspect", m.Image, "--format", `{{ index .Config.Labels "org.opencontainers.image.revision" }} {{ index .Config.Labels "ai.atuin.core.revision" }} {{ index .Config.Labels "ai.atuin.source.archive" }} {{ index .Config.Labels "ai.atuin.build.recipe" }}`)
	if labels != strings.Join([]string{m.Server, m.Core, m.Archive, m.Recipe}, " ") {
		t.Fatal("source/recipe image labels differ (local consistency, not producer attestation)")
	}
	root := t.TempDir()
	if e = os.Chmod(root, 0755); e != nil {
		t.Fatal(e)
	}
	fake := filepath.Join(root, "fakecc")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", fake, "./fakecc")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("fake build: %v %s", e, out)
	}
	casesPath, e := filepath.Abs("../../testdata/compat/phase8.json")
	if e != nil {
		t.Fatal(e)
	}
	cases, e := compatfixture.Load(casesPath)
	if e != nil {
		t.Fatal(e)
	}
	owned := startLease(t)
	networkName := "atuin-phase8-" + owned.id
	args := append([]string{"network", "create", "--internal"}, owned.labels()...)
	args = append(args, networkName)
	network := owned.create(t, args...)
	args = append([]string{"run", "-d", "--name", networkName + "-fake", "--network", network, "--network-alias", "fakecc", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--user=65534:65534", "--memory=64m", "--cpus=1", "--pids-limit=32"}, owned.labels()...)
	args = append(args, "--mount", "type=bind,src="+fake+",dst=/fakecc,readonly", "--mount", "type=bind,src="+casesPath+",dst=/cases.json,readonly", alpine, "/fakecc", "/cases.json")
	fakeID := owned.create(t, args...)
	config := filepath.Join(root, "config.toml")
	if e = os.WriteFile(config, []byte("port = 8080\nendpoint = \"http://fakecc:8081/v1\"\ndefault_model = \"gpt-6.1-sol\"\n[[models]]\nalias = \"gpt-6.1-sol\"\nname = \"Sol\"\ndescription = \"\"\nmodel = \"gpt-6.1-sol\"\n"), 0644); e != nil {
		t.Fatal(e)
	}
	args = append([]string{"run", "-d", "--name", networkName + "-oracle", "--network", network, "--network-alias", "oracle", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--memory=256m", "--cpus=2", "--pids-limit=128"}, owned.labels()...)
	args = append(args, "--mount", "type=bind,src="+config+",dst=/etc/atuin-ai/config.toml,readonly", m.Image)
	oracle := owned.create(t, args...)
	const base = "http://oracle:8080"
	if docker(t, "port", oracle) != "" || docker(t, "port", fakeID) != "" {
		t.Fatal("oracle must have NO published ports")
	}
	deadline := time.Now().Add(20 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		status, _, _ := oracleRequest(t, fakeID, base+"/api/cli/models", nil)
		if status == 200 {
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("oracle not ready: %s", docker(t, "logs", oracle))
	}
	turns := []testutil.Turn{}
	for _, c := range cases {
		turns = append(turns, c.Turn)
	}
	n := testutil.New(t, turns, true)
	var originalModels, nativeModels any
	_, modelBody, _ := oracleRequest(t, fakeID, base+"/api/cli/models", nil)
	if json.Unmarshal([]byte(modelBody), &originalModels) != nil || json.Unmarshal(get(t, n.URL, "/api/cli/models"), &nativeModels) != nil || !reflect.DeepEqual(originalModels, nativeModels) {
		t.Fatal("exact model list/default differs")
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			status, body, header := testutil.PostWithSession(t, n.URL, c.Request)
			refStatus, refBody, refHeader := oracleRequest(t, fakeID, base+"/api/cli/chat", c.Request)
			if status != 200 || refStatus != 200 {
				t.Fatalf("HTTP differs: %d/%d", status, refStatus)
			}
			native, e := compatfixture.Events(body)
			if e != nil {
				t.Fatal(e)
			}
			reference, e := compatfixture.Events(refBody)
			if e != nil {
				t.Fatal(e)
			}
			if e = compareCase(c, native, header, reference, refHeader); e != nil {
				t.Fatal(e)
			}
		})
	}
	client, up, _, _, _ := n.Captures()
	if len(client) != len(cases) || len(up) != len(cases) {
		t.Fatal("native generation/request count differs")
	}
	var counts map[string]int
	_, countBody, _ := oracleRequest(t, fakeID, "http://127.0.0.1:8081/counts", nil)
	if json.Unmarshal([]byte(countBody), &counts) != nil {
		t.Fatal("fake counts malformed")
	}
	if t.Failed() {
		t.Logf("CC counts: %v; original diagnostics: %s", counts, docker(t, "logs", oracle))
	}
	if counts["__received"] != len(cases) || counts["__validated"] != len(cases) || counts["__invalid"] != 0 || counts["__unknown"] != 0 {
		t.Fatalf("unexpected/rejected CC requests: %v", counts)
	}
	for _, c := range cases {
		if counts[c.Name] != 1 {
			t.Fatalf("reference generation count %s=%d", c.Name, counts[c.Name])
		}
	}
}
