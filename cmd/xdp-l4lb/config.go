package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Services []ServiceConfig `yaml:"services"`
}

type ServiceConfig struct {
	Name     string          `yaml:"name"`
	VIP      string          `yaml:"vip"`
	Port     uint16          `yaml:"port"`
	Protocol string          `yaml:"protocol"`
	Mode     string          `yaml:"mode"`
	Backends []BackendConfig `yaml:"backends"`
}

type BackendConfig struct {
	Name    string `yaml:"name"`
	IP      string `yaml:"ip"`
	MAC     string `yaml:"mac"`
	IfName  string `yaml:"ifname"`
	IfIndex int    `yaml:"ifindex"`
}

func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)

	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("config must contain exactly one yaml document")
		}
		return nil, fmt.Errorf("decode trailing config data: %w", err)
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func validateConfig(cfg *Config) error {
	if len(cfg.Services) == 0 {
		return fmt.Errorf("at least one service is required")
	}
	if len(cfg.Services) > maxServices {
		return fmt.Errorf("service count %d exceeds limit %d", len(cfg.Services), maxServices)
	}

	serviceNames := make(map[string]struct{}, len(cfg.Services))
	serviceKeys := make(map[string]struct{}, len(cfg.Services))
	totalBackends := 0

	for si := range cfg.Services {
		svc := &cfg.Services[si]
		svc.VIP = strings.TrimSpace(svc.VIP)
		svc.Protocol = strings.ToLower(strings.TrimSpace(svc.Protocol))
		svc.Mode = strings.ToLower(strings.TrimSpace(svc.Mode))
		svc.Name = strings.TrimSpace(svc.Name)

		vip := net.ParseIP(svc.VIP).To4()
		if vip == nil {
			return fmt.Errorf("service[%d] has invalid ipv4 vip %q", si, svc.VIP)
		}
		svc.VIP = net.IP(vip).String()
		if svc.Port == 0 {
			return fmt.Errorf("service[%d] port must be non-zero", si)
		}
		switch svc.Protocol {
		case "tcp", "udp":
		default:
			return fmt.Errorf("service[%d] protocol must be tcp or udp", si)
		}
		if svc.Mode == "" {
			svc.Mode = "dsr"
		}
		if svc.Mode != "dsr" {
			return fmt.Errorf("service[%d] mode must be dsr", si)
		}
		if svc.Name == "" {
			svc.Name = fmt.Sprintf("%s:%d/%s", svc.VIP, svc.Port, svc.Protocol)
		}
		if _, exists := serviceNames[svc.Name]; exists {
			return fmt.Errorf("service[%d] duplicates service name %q", si, svc.Name)
		}
		serviceNames[svc.Name] = struct{}{}

		key := fmt.Sprintf("%s:%d/%s", svc.VIP, svc.Port, svc.Protocol)
		if _, exists := serviceKeys[key]; exists {
			return fmt.Errorf("service[%d] duplicates service key %s", si, key)
		}
		serviceKeys[key] = struct{}{}

		if len(svc.Backends) == 0 {
			return fmt.Errorf("service[%d] must define at least one backend", si)
		}
		if totalBackends > maxBackends-len(svc.Backends) {
			return fmt.Errorf("backend count exceeds limit %d", maxBackends)
		}
		totalBackends += len(svc.Backends)

		backendNames := make(map[string]struct{}, len(svc.Backends))
		for bi := range svc.Backends {
			backend := &svc.Backends[bi]
			backend.Name = strings.TrimSpace(backend.Name)
			backend.IP = strings.TrimSpace(backend.IP)
			backend.MAC = strings.TrimSpace(backend.MAC)
			backend.IfName = strings.TrimSpace(backend.IfName)

			backendIP := net.ParseIP(backend.IP).To4()
			if backendIP == nil {
				return fmt.Errorf("service[%d].backend[%d] has invalid ipv4 ip %q", si, bi, backend.IP)
			}
			backend.IP = net.IP(backendIP).String()

			mac, err := net.ParseMAC(backend.MAC)
			if err != nil || len(mac) != 6 {
				return fmt.Errorf("service[%d].backend[%d] has invalid ethernet mac %q", si, bi, backend.MAC)
			}
			if isZeroMAC(mac) || mac[0]&1 != 0 {
				return fmt.Errorf("service[%d].backend[%d] mac must be a non-zero unicast address", si, bi)
			}
			backend.MAC = mac.String()

			if backend.IfIndex < 0 {
				return fmt.Errorf("service[%d].backend[%d] ifindex must be positive", si, bi)
			}
			if backend.IfIndex == 0 && backend.IfName == "" {
				return fmt.Errorf("service[%d].backend[%d] needs ifname or ifindex", si, bi)
			}

			if backend.Name == "" {
				backend.Name = backend.IP
			}
			if _, exists := backendNames[backend.Name]; exists {
				return fmt.Errorf("service[%d].backend[%d] duplicates backend name %q", si, bi, backend.Name)
			}
			backendNames[backend.Name] = struct{}{}
		}
	}

	return nil
}

func isZeroMAC(mac net.HardwareAddr) bool {
	for _, octet := range mac {
		if octet != 0 {
			return false
		}
	}
	return true
}
