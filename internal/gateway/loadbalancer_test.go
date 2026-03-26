package gateway

import (
	"testing"
)

func TestLoadBalancer_RoundRobin(t *testing.T) {
	lb := NewLoadBalancer([]string{
		"http://localhost:8081",
		"http://localhost:8082",
		"http://localhost:8083",
	})

	if len(lb.Targets()) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(lb.Targets()))
	}

	// Track which targets are hit
	hits := make(map[string]int)
	for i := 0; i < 6; i++ {
		target := lb.NextTarget()
		hits[target.URL.String()]++
	}

	// Each target should be hit twice
	for url, count := range hits {
		if count != 2 {
			t.Errorf("expected target %s to be hit 2 times, got %d", url, count)
		}
	}
}

func TestLoadBalancer_SkipsDeadTargets(t *testing.T) {
	lb := NewLoadBalancer([]string{
		"http://localhost:8081",
		"http://localhost:8082",
	})

	// Mark first target as dead
	lb.Targets()[0].SetAlive(false)

	// All requests should go to the alive target
	for i := 0; i < 5; i++ {
		target := lb.NextTarget()
		if target.URL.String() != "http://localhost:8082" {
			t.Errorf("expected requests to go to alive target, got %s", target.URL.String())
		}
	}
}

func TestLoadBalancer_EmptyTargets(t *testing.T) {
	lb := NewLoadBalancer([]string{})

	target := lb.NextTarget()
	if target != nil {
		t.Error("expected nil for empty targets")
	}
}

func TestTarget_AliveState(t *testing.T) {
	lb := NewLoadBalancer([]string{"http://localhost:8081"})
	target := lb.Targets()[0]

	if !target.IsAlive() {
		t.Error("new target should be alive")
	}

	target.SetAlive(false)
	if target.IsAlive() {
		t.Error("target should be dead after SetAlive(false)")
	}

	target.SetAlive(true)
	if !target.IsAlive() {
		t.Error("target should be alive after SetAlive(true)")
	}
}
