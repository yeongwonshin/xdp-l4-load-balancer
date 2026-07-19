package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigNormalizesValues(t *testing.T) {
	path := writeTestConfig(t, `
services:
  - vip: 10.10.0.100
    port: 80
    protocol: TCP
    backends:
      - ip: 10.10.0.11
        mac: "02:42:0A:0A:00:0B"
        ifname: eth0
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	service := cfg.Services[0]
	if service.Name != "10.10.0.100:80/tcp" {
		t.Fatalf("unexpected generated service name: %q", service.Name)
	}
	if service.Protocol != "tcp" || service.Mode != "dsr" {
		t.Fatalf("service was not normalized: protocol=%q mode=%q", service.Protocol, service.Mode)
	}
	backend := service.Backends[0]
	if backend.Name != "10.10.0.11" {
		t.Fatalf("unexpected generated backend name: %q", backend.Name)
	}
	if backend.MAC != "02:42:0a:0a:00:0b" {
		t.Fatalf("backend mac was not normalized: %q", backend.MAC)
	}
}

func TestLoadConfigRejectsUnknownField(t *testing.T) {
	path := writeTestConfig(t, `
services:
  - name: web
    vip: 10.10.0.100
    port: 80
    protocol: tcp
    unexpected: true
    backends:
      - name: web-1
        ip: 10.10.0.11
        mac: "02:42:0a:0a:00:0b"
        ifname: eth0
`)

	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "field unexpected not found") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestLoadConfigRejectsDuplicateServiceKey(t *testing.T) {
	path := writeTestConfig(t, `
services:
  - name: web-a
    vip: 10.10.0.100
    port: 80
    protocol: tcp
    backends:
      - name: web-1
        ip: 10.10.0.11
        mac: "02:42:0a:0a:00:0b"
        ifname: eth0
  - name: web-b
    vip: 10.10.0.100
    port: 80
    protocol: TCP
    backends:
      - name: web-2
        ip: 10.10.0.12
        mac: "02:42:0a:0a:00:0c"
        ifname: eth0
`)

	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "duplicates service key") {
		t.Fatalf("expected duplicate service key error, got %v", err)
	}
}

func TestLoadConfigRejectsMulticastMAC(t *testing.T) {
	path := writeTestConfig(t, `
services:
  - name: web
    vip: 10.10.0.100
    port: 80
    protocol: tcp
    backends:
      - name: web-1
        ip: 10.10.0.11
        mac: "01:00:5e:00:00:01"
        ifname: eth0
`)

	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "non-zero unicast") {
		t.Fatalf("expected multicast mac error, got %v", err)
	}
}

func TestLoadConfigRejectsMultipleDocuments(t *testing.T) {
	path := writeTestConfig(t, `
services:
  - name: web
    vip: 10.10.0.100
    port: 80
    protocol: tcp
    backends:
      - name: web-1
        ip: 10.10.0.11
        mac: "02:42:0a:0a:00:0b"
        ifname: eth0
---
services: []
`)

	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "exactly one yaml document") {
		t.Fatalf("expected multiple document error, got %v", err)
	}
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestValidateConfigNormalizesValues(t *testing.T) {
	cfg := Config{Services: []ServiceConfig{{
		VIP:      "::ffff:10.10.0.100",
		Port:     80,
		Protocol: " TCP ",
		Backends: []BackendConfig{{
			IP:     "::ffff:10.10.0.11",
			MAC:    "02:42:0A:0A:00:0B",
			IfName: " eth0 ",
		}},
	}}}

	if err := validateConfig(&cfg); err != nil {
		t.Fatalf("validateConfig returned error: %v", err)
	}
	service := cfg.Services[0]
	if service.VIP != "10.10.0.100" || service.Protocol != "tcp" || service.Mode != "dsr" {
		t.Fatalf("service was not normalized: %#v", service)
	}
	backend := service.Backends[0]
	if backend.IP != "10.10.0.11" || backend.IfName != "eth0" || backend.MAC != "02:42:0a:0a:00:0b" {
		t.Fatalf("backend was not normalized: %#v", backend)
	}
}

func TestValidateConfigRejectsEquivalentDuplicateServiceKeys(t *testing.T) {
	cfg := Config{Services: []ServiceConfig{
		{
			Name:     "web-a",
			VIP:      "10.10.0.100",
			Port:     80,
			Protocol: "tcp",
			Backends: []BackendConfig{{Name: "web-a-1", IP: "10.10.0.11", MAC: "02:42:0a:0a:00:0b", IfName: "eth0"}},
		},
		{
			Name:     "web-b",
			VIP:      "::ffff:10.10.0.100",
			Port:     80,
			Protocol: "TCP",
			Backends: []BackendConfig{{Name: "web-b-1", IP: "10.10.0.12", MAC: "02:42:0a:0a:00:0c", IfName: "eth0"}},
		},
	}}

	err := validateConfig(&cfg)
	if err == nil || !strings.Contains(err.Error(), "duplicates service key") {
		t.Fatalf("expected duplicate service key error, got %v", err)
	}
}
