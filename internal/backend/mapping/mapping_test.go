package mapping

import (
	"errors"
	"testing"

	"github.com/HMasataka/cloudia/pkg/models"
)

// --- AMI ---

func TestResolveAMI_registered(t *testing.T) {
	m := map[string]string{
		"ami-ubuntu-22.04": "ami-0c02fb55956c7d316",
	}
	got, err := resolveAMI("ami-ubuntu-22.04", m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ami-0c02fb55956c7d316" {
		t.Errorf("got %q, want %q", got, "ami-0c02fb55956c7d316")
	}
}

func TestResolveAMI_notFound(t *testing.T) {
	m := map[string]string{}
	_, err := resolveAMI("ami-unknown", m)
	if !errors.Is(err, models.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestResolveAMI_defaultMap(t *testing.T) {
	cases := []struct {
		amiID string
		want  string
	}{
		{"ami-ubuntu-22.04", "ami-0c02fb55956c7d316"},
		{"ami-amazon-linux2", "ami-0c55b159cbfafe1f0"},
		{"ami-debian-11", "ami-09a41e26df96c4aef"},
	}
	for _, tc := range cases {
		got, err := ResolveAMI(tc.amiID)
		if err != nil {
			t.Errorf("ResolveAMI(%q): unexpected error: %v", tc.amiID, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ResolveAMI(%q) = %q, want %q", tc.amiID, got, tc.want)
		}
	}
}

// --- MachineType ---

func TestResolveMachineType_registered(t *testing.T) {
	m := map[string]MachineSpec{
		"t2.micro": {CPU: 1_000_000_000, Memory: 1_073_741_824},
	}
	got, err := resolveMachineType("t2.micro", m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.CPU != 1_000_000_000 {
		t.Errorf("CPU: got %d, want %d", got.CPU, 1_000_000_000)
	}
	if got.Memory != 1_073_741_824 {
		t.Errorf("Memory: got %d, want %d", got.Memory, 1_073_741_824)
	}
}

func TestResolveMachineType_notFound(t *testing.T) {
	m := map[string]MachineSpec{}
	_, err := resolveMachineType("x2.unknown", m)
	if !errors.Is(err, models.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestResolveMachineType_defaultMap(t *testing.T) {
	cases := []struct {
		machineType string
		wantCPU     int64
		wantMemory  int64
	}{
		{"t2.micro", 1_000_000_000, 1_073_741_824},
		{"t2.small", 1_000_000_000, 2_147_483_648},
		{"t2.medium", 2_000_000_000, 4_294_967_296},
		{"n1-standard-1", 1_000_000_000, 3_840_000_000},
	}
	for _, tc := range cases {
		got, err := ResolveMachineType(tc.machineType)
		if err != nil {
			t.Errorf("ResolveMachineType(%q): unexpected error: %v", tc.machineType, err)
			continue
		}
		if got.CPU != tc.wantCPU {
			t.Errorf("ResolveMachineType(%q).CPU = %d, want %d", tc.machineType, got.CPU, tc.wantCPU)
		}
		if got.Memory != tc.wantMemory {
			t.Errorf("ResolveMachineType(%q).Memory = %d, want %d", tc.machineType, got.Memory, tc.wantMemory)
		}
	}
}

// --- AMI Docker ---

func TestResolveDockerImage_known(t *testing.T) {
	cases := []struct {
		amiID string
		want  string
	}{
		{"ami-0c02fb55956c7d316", "ubuntu:22.04"},
		{"ami-0c55b159cbfafe1f0", "amazonlinux:2"},
		{"ami-09a41e26df96c4aef", "debian:11"},
	}
	for _, tc := range cases {
		got := ResolveDockerImage(tc.amiID)
		if got != tc.want {
			t.Errorf("ResolveDockerImage(%q) = %q, want %q", tc.amiID, got, tc.want)
		}
	}
}

func TestResolveDockerImage_unknown(t *testing.T) {
	got := ResolveDockerImage("ami-unknown-xxx")
	if got != "ubuntu:22.04" {
		t.Errorf("ResolveDockerImage(unknown) = %q, want %q", got, "ubuntu:22.04")
	}
}

// --- Runtime ---

func TestResolveRuntime_registered(t *testing.T) {
	m := map[string]string{
		"python3.11": "python:3.11-slim",
	}
	got, err := resolveRuntime("python3.11", m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "python:3.11-slim" {
		t.Errorf("got %q, want %q", got, "python:3.11-slim")
	}
}

func TestResolveRuntime_notFound(t *testing.T) {
	m := map[string]string{}
	_, err := resolveRuntime("ruby3.2", m)
	if !errors.Is(err, models.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestResolveRuntime_defaultMap(t *testing.T) {
	cases := []struct {
		runtime string
		want    string
	}{
		{"python3.11", "python:3.11-slim"},
		{"node18", "node:18-alpine"},
		{"go1.21", "golang:1.21-alpine"},
	}
	for _, tc := range cases {
		got, err := ResolveRuntime(tc.runtime)
		if err != nil {
			t.Errorf("ResolveRuntime(%q): unexpected error: %v", tc.runtime, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ResolveRuntime(%q) = %q, want %q", tc.runtime, got, tc.want)
		}
	}
}
