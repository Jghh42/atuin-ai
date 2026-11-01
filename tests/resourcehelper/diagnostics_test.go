package main

import (
	"bytes"
	"encoding/json"
	"runtime/metrics"
	"testing"
)

func TestCompanionReadinessRequiresStartupPriming(t *testing.T) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	unprimed := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}, {Name: "/gc/heap/objects:objects"}, {Name: "/sched/goroutines:goroutines"}}
	if companionReady(encoder, unprimed) == nil || out.Len() != 0 {
		t.Fatal("READY escaped before startup telemetry priming")
	}
	if companionReady(encoder, newDiagnosticSamples()) != nil || out.String() != "{\"companion_ready\":true}\n" {
		t.Fatal("primed readiness contract changed")
	}
}
func TestCompanionTelemetryInitializedBeforeBaseline(t *testing.T) {
	samples := newDiagnosticSamples()
	want := []string{"/memory/classes/heap/objects:bytes", "/gc/heap/objects:objects", "/sched/goroutines:goroutines"}
	if len(samples) != len(want) {
		t.Fatal("diagnostic sample contract changed")
	}
	for j, name := range want {
		if samples[j].Name != name || samples[j].Value.Kind() != metrics.KindUint64 || samples[j].Value.Uint64() == 0 {
			t.Fatal("telemetry was not initialized before readiness/baseline")
		}
	}
	first := &samples[0]
	metrics.Read(samples)
	if &samples[0] != first {
		t.Fatal("diagnostic reads replaced baseline sample storage")
	}
}
