package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/continuation"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/prompt"
	"github.com/Jghh42/atuin-ai/internal/provider"
	"github.com/Jghh42/atuin-ai/internal/tools"
)

type replayRun struct {
	privateValue
	*replayRunPayload
}
type replayRunPayload struct {
	end   int
	items []json.RawMessage
}

func (*replayRun) LogValue() slog.Value { return slog.StringValue("[private generation]") }

type preparedClient struct {
	privateValue
	*preparedClientPayload
}
type preparedClientPayload struct {
	native  RequestClient
	manager *credentials.Manager
	store   *continuation.Store
}

func (*preparedClient) LogValue() slog.Value { return slog.StringValue("[private generation]") }

// NewPrepared is the production composition constructor. Direct Stream remains
// the legacy fail-closed path; reasoning can only reach the private transaction.
func NewPrepared(native RequestClient, manager *credentials.Manager, store *continuation.Store) provider.Provider {
	native.Credentials = manager
	return &preparedClient{preparedClientPayload: &preparedClientPayload{native: native, manager: manager, store: store}}
}
func (c *preparedClient) Stream(ctx context.Context, r provider.Request, sink func(provider.Event) error) (provider.Usage, error) {
	return c.native.Stream(ctx, r, sink)
}

var _ provider.PreparedProvider = (*preparedClient)(nil)

type preparedGeneration struct {
	privateValue
	*preparedGenerationPayload
}
type preparedGenerationPayload struct {
	mu                                    sync.Mutex
	once                                  sync.Once
	client                                *preparedClient
	original                              context.Context
	identity                              credentials.Identity
	lease                                 *continuation.Lease
	request                               atuin.Request
	instructions                          string
	definitions                           []functionDefinition
	definitionBytes                       []byte
	replay                                map[int]replayRun
	prior, next                           continuation.Record
	scope                                 continuation.Commitment
	candidate                             *completion
	calls                                 []tools.Call
	started, completed, closed, published bool
}

func (*preparedGeneration) LogValue() slog.Value { return slog.StringValue("[private generation]") }

func continuationFailure(err error) error {
	category := provider.ContinuationUnavailable
	if typed, ok := err.(continuation.Error); ok {
		switch typed.Code() {
		case continuation.HandleInvalid:
			category = provider.ContinuationHandleInvalid
		case continuation.Missing:
			category = provider.ContinuationMissing
		case continuation.Expired:
			category = provider.ContinuationExpired
		case continuation.Corrupt:
			category = provider.ContinuationCorrupt
		case continuation.BindingMismatch:
			category = provider.ContinuationBindingMismatch
		case continuation.Limit:
			category = provider.ContinuationLimit
		case continuation.ClockInvalid:
			category = provider.ContinuationClockInvalid
		case continuation.CommitFailed:
			category = provider.ContinuationCommitFailed
		case continuation.LifecycleChanged:
			category = provider.ContinuationLifecycleChanged
		case continuation.ProjectionUnsupported:
			category = provider.ContinuationProjectionUnsupported
		}
	}
	return failure(ErrRequest, provider.PhaseContinuation, category)
}
func localFailure(category provider.Category) error {
	return failure(ErrRequest, provider.PhaseContinuation, category)
}
func copyOptional(p *string) *string {
	if p == nil {
		return nil
	}
	s := *p
	return &s
}
func ownRequest(r atuin.Request) atuin.Request {
	r.InvocationID = nil
	r.Messages = append([]atuin.Message(nil), r.Messages...)
	for j := range r.Messages {
		r.Messages[j].Content = append([]atuin.Block(nil), r.Messages[j].Content...)
		for n := range r.Messages[j].Content {
			b := &r.Messages[j].Content[n]
			b.Input = append(json.RawMessage(nil), b.Input...)
			if b.ContentLength != nil {
				v := *b.ContentLength
				b.ContentLength = &v
			}
		}
	}
	r.Context.OS = copyOptional(r.Context.OS)
	r.Context.Distro = copyOptional(r.Context.Distro)
	r.Context.Shell = copyOptional(r.Context.Shell)
	r.Context.PreferredLanguage = copyOptional(r.Context.PreferredLanguage)
	r.Context.PWD = copyOptional(r.Context.PWD)
	r.Context.LastCommand = copyOptional(r.Context.LastCommand)
	caps := make(atuin.Capabilities, len(r.Config.Capabilities))
	for k, v := range r.Config.Capabilities {
		caps[k] = v
	}
	r.Config.Capabilities = caps
	r.Config.UserContexts = append([]atuin.UserContext(nil), r.Config.UserContexts...)
	r.Config.Skills = append([]atuin.Skill(nil), r.Config.Skills...)
	r.Config.SkillsOverflow = copyOptional(r.Config.SkillsOverflow)
	return r
}
func (c *preparedClient) Prepare(ctx context.Context, request provider.Request) (provider.PreparedGeneration, error) {
	r := request.Conversation
	if !bounded(r) {
		return nil, failure(ErrRequest, provider.PhaseRequest, provider.RequestInvalid)
	}
	marked, e := continuation.Marked(r.Messages)
	if e != nil {
		return nil, continuationFailure(e)
	}
	if c.manager == nil || c.store == nil {
		return nil, localFailure(provider.ContinuationUnavailable)
	}
	credential, e := c.manager.Current(ctx)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if e != nil || !credentials.MatchesIdentity(ctx, credential.Identity) {
		return nil, failure(ErrCredentials, provider.PhaseCredentials, provider.CredentialsUnavailable)
	}
	// Pin identity only; no access token is retained in the generation.
	identity := credential.Identity
	credential = credentials.Credential{}
	original := credentials.BindIdentity(ctx, identity)
	r = ownRequest(r)
	lease, e := c.store.Acquire(original, r.Messages)
	if e != nil {
		return nil, continuationFailure(e)
	}
	g := &preparedGeneration{preparedGenerationPayload: &preparedGenerationPayload{client: c, original: original, identity: identity, lease: lease, request: r, instructions: prompt.Build(r.Context, r.Config), definitions: functionDefinitions(r), replay: map[int]replayRun{}}}
	definitions, e := json.Marshal(struct {
		Tools    []functionDefinition `json:"tools"`
		Choice   string               `json:"tool_choice"`
		Parallel bool                 `json:"parallel_tool_calls"`
	}{g.definitions, "auto", true})
	if e != nil {
		g.Close()
		return nil, failure(ErrRequest, provider.PhaseRequest, provider.RequestInvalid)
	}
	g.definitionBytes = definitions
	if marked {
		policy, e := lease.Policy(r, []byte(g.instructions), definitions)
		if e != nil {
			g.Close()
			return nil, continuationFailure(e)
		}
		scope, e := lease.Scope(continuation.NewScopeInput(identity, r.Config.ProviderModel, r.SessionID, policy))
		if e != nil {
			g.Close()
			return nil, continuationFailure(e)
		}
		g.scope = scope
		prior, matches, e := lease.Load(r.Messages, scope)
		if e != nil {
			g.Close()
			return nil, continuationFailure(e)
		}
		g.prior = prior
		for index, match := range matches {
			start, end := match.Bounds()
			items, e := prior.ReplayItems(index, match, r.Messages)
			if e != nil {
				g.Close()
				return nil, continuationFailure(e)
			}
			g.replay[start] = replayRun{replayRunPayload: &replayRunPayload{end: end, items: items}}
		}
	}
	if ctx.Err() != nil || lease.Check() != nil {
		g.Close()
		return nil, localFailure(provider.ContinuationLifecycleChanged)
	}
	return g, nil
}
func (g *preparedGeneration) Stream(ctx context.Context, sink func(provider.Event) error) (provider.Usage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if sink == nil {
		return provider.Usage{}, failure(ErrRequest, provider.PhaseRequest, provider.RequestInvalid)
	}
	if g.closed || g.started || ctx.Err() != nil || g.lease.Check() != nil {
		return provider.Usage{}, localFailure(provider.ContinuationLifecycleChanged)
	}
	g.started = true
	if e := g.lease.MarkStreamStarted(); e != nil {
		return provider.Usage{}, continuationFailure(e)
	}
	// This defer runs ONLY after streamNative's body-close and watchdog joins.
	defer g.lease.StreamDone()
	var outbound []byte
	defer func() { clear(outbound) }()
	open := func(nativeCtx context.Context) (*http.Response, error) {
		now := time.Now()
		if g.client.native.Now != nil {
			now = g.client.native.Now()
		}
		body, e := projectRequest(g.request, now, g.instructions, g.definitions, g.replay)
		if e != nil {
			return nil, failure(ErrRequest, provider.PhaseRequest, provider.RequestInvalid)
		}
		outbound = body
		return g.client.native.openEncoded(nativeCtx, body)
	}
	parse := func(r io.Reader, emit func(provider.Event) error) (provider.Usage, error) {
		u, c, e := parseConfigured(r, emit, true)
		g.candidate = c
		return u, e
	}
	usage, e := streamNative(g.lease.Context(), open, parse, func(event provider.Event) error {
		if g.lease.Check() != nil {
			return localFailure(provider.ContinuationLifecycleChanged)
		}
		if event.Kind == provider.ToolCalls {
			for _, call := range event.Calls {
				call.Input = append(json.RawMessage(nil), call.Input...)
				g.calls = append(g.calls, call)
			}
		}
		return sink(event)
	}, config.GenerationTimeout, config.UpstreamInactivityTimeout)
	if e != nil {
		if g.original.Err() == nil && g.lease.Check() != nil {
			return provider.Usage{}, localFailure(provider.ContinuationLifecycleChanged)
		}
		return provider.Usage{}, e
	}
	g.completed = true
	return usage, nil
}
func (g *preparedGeneration) Publish(ctx context.Context, batch tools.Batch, emit func(context.Context, []tools.Call) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if emit == nil || g.closed || g.published || !g.completed || g.candidate == nil || ctx.Err() != nil || g.lease.Check() != nil {
		return localFailure(provider.ContinuationLifecycleChanged)
	}
	if len(batch.Calls) != len(g.calls) {
		return localFailure(provider.ContinuationBindingMismatch)
	}
	history := map[string]bool{}
	backend := map[string]bool{}
	for _, m := range g.request.Messages {
		for _, b := range m.Content {
			if b.Type == "tool_use" {
				history[b.ID] = true
				if !continuation.Reserved(b.ID) {
					backend[b.ID] = true
				}
			}
		}
	}
	for _, id := range g.prior.BackendIDs() {
		backend[id] = true
	}
	for j, call := range batch.Calls {
		original := g.calls[j]
		if call.ID != original.ID || call.Name != original.Name || batch.Kind != tools.Suggestion && !bytes.Equal(call.Input, original.Input) {
			return localFailure(provider.ContinuationBindingMismatch)
		}
		if continuation.Reserved(call.ID) || backend[call.ID] || history[call.ID] {
			return localFailure(provider.HistoryCallIDCollision)
		}
	}
	required := batch.Kind == tools.LocalTools && len(batch.Calls) > 0 && (g.candidate.handled || !g.prior.Empty())
	translated := make([]tools.Call, len(batch.Calls))
	copy(translated, batch.Calls)
	if required {
		if g.candidate.missingState {
			return localFailure(provider.ReasoningStateMissing)
		}
		if g.candidate.projectionUnsupported {
			return localFailure(provider.ContinuationProjectionUnsupported)
		}
		for _, call := range batch.Calls {
			canonical, e := continuation.CanonicalArguments(call.Input, true)
			clear(canonical)
			if e != nil {
				return continuationFailure(e)
			}
		}
		if g.prior.Empty() {
			policy, e := g.lease.Policy(g.request, []byte(g.instructions), g.definitionBytes)
			if e != nil {
				return continuationFailure(e)
			}
			scope, e := g.lease.Scope(continuation.NewScopeInput(g.identity, g.request.Config.ProviderModel, g.request.SessionID, policy))
			if e != nil {
				return continuationFailure(e)
			}
			g.scope = scope
		}
		prefix, e := g.lease.Prefix(g.request.Messages)
		if e != nil {
			return continuationFailure(e)
		}
		text, recipe, e := g.lease.AcceptCapture(g.candidate.capture)
		if e != nil {
			return continuationFailure(e)
		}
		calls := make([]continuation.Call, 0, len(batch.Calls))
		for _, call := range batch.Calls {
			calls = append(calls, continuation.ExecutableCall(call.ID, call.Name))
		}
		next, e := g.lease.BuildNext(g.scope, prefix, text, recipe, calls, g.candidate.items, g.prior)
		if e != nil {
			return continuationFailure(e)
		}
		g.next = next
		handles := next.NewHandles()
		for j, id := range handles {
			if history[id] {
				return localFailure(provider.HistoryCallIDCollision)
			}
			history[id] = true
			translated[j].ID = id
		}
		// Revalidate translated IDs/arguments without undoing safety normalization.
		if _, e := tools.ValidateBatch(translated, g.request.Config.Capabilities); e != nil {
			return localFailure(provider.ToolBatchInvalid)
		}
	}
	var emissionError error
	e := g.client.manager.WithIdentityPublication(g.lease.Context(), g.identity, func(p credentials.PublicationLease) error {
		callback := func(original context.Context) error { emissionError = emit(original, translated); return emissionError }
		if required {
			_, e := g.lease.CommitAndPublish(ctx, p, g.next, callback)
			return e
		}
		return g.lease.PublishWithoutRecord(ctx, p, g.prior, callback)
	})
	if emissionError != nil {
		return emissionError
	}
	if e != nil {
		if _, ok := e.(continuation.Error); ok {
			return continuationFailure(e)
		}
		return localFailure(provider.ContinuationLifecycleChanged)
	}
	g.published = true
	return nil
}
func (g *preparedGeneration) Outcome() provider.GenerationOutcome {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.published || g.candidate == nil || g.lease.Check() != nil {
		return provider.GenerationOutcome{}
	}
	return provider.NewGenerationOutcome(g.candidate.handled, !g.prior.Empty())
}
func (g *preparedGeneration) Close() {
	if g == nil || g.preparedGenerationPayload == nil {
		return
	}
	g.once.Do(func() {
		g.lease.Close()
		g.mu.Lock()
		defer g.mu.Unlock()
		g.closed = true
		g.prior.Clear()
		g.next.Clear()
		if g.candidate != nil {
			g.candidate.close()
		}
		for _, call := range g.calls {
			clear(call.Input)
		}
		for _, m := range g.request.Messages {
			for _, b := range m.Content {
				clear(b.Input)
			}
		}
		for _, run := range g.replay {
			for _, raw := range run.items {
				clear(raw)
			}
		}
		clear(g.definitionBytes)
		g.request = atuin.Request{}
		g.instructions = ""
		g.definitions = nil
		g.replay = nil
		g.calls = nil
	})
}
