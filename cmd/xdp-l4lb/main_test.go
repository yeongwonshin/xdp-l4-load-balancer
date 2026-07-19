package main

import (
	"testing"

	"github.com/cilium/ebpf/link"
)

func TestParseXDPModeNormalizesInput(t *testing.T) {
	tests := map[string]link.XDPAttachFlags{
		"generic":  link.XDPGenericMode,
		" DRIVER ": link.XDPDriverMode,
		"Hw":       link.XDPOffloadMode,
	}

	for input, want := range tests {
		got, err := parseXDPMode(input)
		if err != nil {
			t.Fatalf("parseXDPMode %q returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("parseXDPMode %q = %v, want %v", input, got, want)
		}
	}
}

func TestParseXDPModeRejectsUnknownMode(t *testing.T) {
	if _, err := parseXDPMode("native"); err == nil {
		t.Fatal("expected unknown mode error")
	}
}
