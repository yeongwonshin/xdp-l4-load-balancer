package main

import (
	"fmt"
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
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}

	if len(cfg.Services) == 0 {
		return nil, fmt.Errorf("at least one service is required")
	}

	for si, svc := range cfg.Services {
		if net.ParseIP(svc.VIP).To4() == nil {
			return nil, fmt.Errorf("service[%d] has invalid IPv4 VIP %q", si, svc.VIP)
		}
		if svc.Port == 0 {
			return nil, fmt.Errorf("service[%d] port must be non-zero", si)
		}
		switch strings.ToLower(svc.Protocol) {
		case "tcp", "udp":
		default:
			return nil, fmt.Errorf("service[%d] protocol must be tcp or udp", si)
		}
		if svc.Mode == "" {
			cfg.Services[si].Mode = "dsr"
		}
		if strings.ToLower(cfg.Services[si].Mode) != "dsr" {
			return nil, fmt.Errorf("service[%d] only dsr mode is implemented in this skeleton", si)
		}
		if len(svc.Backends) == 0 {
			return nil, fmt.Errorf("service[%d] must define at least one backend", si)
		}
		for bi, be := range svc.Backends {
			if net.ParseIP(be.IP).To4() == nil {
				return nil, fmt.Errorf("service[%d].backend[%d] has invalid IPv4 IP %q", si, bi, be.IP)
			}
			if _, err := net.ParseMAC(be.MAC); err != nil {
				return nil, fmt.Errorf("service[%d].backend[%d] has invalid MAC %q: %w", si, bi, be.MAC, err)
			}
			if be.IfIndex == 0 && be.IfName == "" {
				return nil, fmt.Errorf("service[%d].backend[%d] needs ifname or ifindex", si, bi)
			}
		}
	}

	return &cfg, nil
}
