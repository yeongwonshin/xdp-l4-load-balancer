package main

import (
	"fmt"
	"net"
	"strings"

	"github.com/cilium/ebpf"
)

type BackendLabel struct {
	ID      uint32
	Service string
	Name    string
	IP      string
}

func ProgramMaps(objs *bpfObjects, cfg *Config) error {
	possibleCPUs, err := ebpf.PossibleCPU()
	if err != nil {
		return fmt.Errorf("detect possible cpus: %w", err)
	}
	zeroStats := make([]backendStats, possibleCPUs)

	var backendID uint32
	for _, svc := range cfg.Services {
		start := backendID
		for _, backend := range svc.Backends {
			value, err := backendToMapValue(backend)
			if err != nil {
				return fmt.Errorf("service %q backend %q: %w", svc.Name, backend.Name, err)
			}
			if err := objs.Backends.Update(backendID, value, ebpf.UpdateAny); err != nil {
				return fmt.Errorf("update backend %d: %w", backendID, err)
			}
			if err := objs.BackendStats.Update(backendID, zeroStats, ebpf.UpdateNoExist); err != nil {
				return fmt.Errorf("initialize backend stats %d: %w", backendID, err)
			}
			backendID++
		}

		key, err := serviceToMapKey(svc)
		if err != nil {
			return fmt.Errorf("service %q: %w", svc.Name, err)
		}
		value := serviceValue{
			BackendStart: start,
			BackendCount: uint32(len(svc.Backends)),
			Flags:        lbFlagDSR,
		}
		if err := objs.Services.Update(key, value, ebpf.UpdateNoExist); err != nil {
			return fmt.Errorf("update service %q: %w", svc.Name, err)
		}
	}
	return nil
}

func serviceToMapKey(svc ServiceConfig) (serviceKey, error) {
	ip := net.ParseIP(svc.VIP).To4()
	if ip == nil {
		return serviceKey{}, fmt.Errorf("invalid vip %q", svc.VIP)
	}

	var protocol uint8
	switch strings.ToLower(svc.Protocol) {
	case "tcp":
		protocol = protoTCP
	case "udp":
		protocol = protoUDP
	default:
		return serviceKey{}, fmt.Errorf("invalid protocol %q", svc.Protocol)
	}

	return serviceKey{
		VIP:   ipv4ToNetworkOrder(ip),
		Port:  portToNetworkOrder(svc.Port),
		Proto: protocol,
	}, nil
}

func backendToMapValue(backend BackendConfig) (backendValue, error) {
	ip := net.ParseIP(backend.IP).To4()
	if ip == nil {
		return backendValue{}, fmt.Errorf("invalid backend ip %q", backend.IP)
	}

	destinationMAC, err := net.ParseMAC(backend.MAC)
	if err != nil || len(destinationMAC) != 6 {
		return backendValue{}, fmt.Errorf("parse backend mac %q", backend.MAC)
	}

	egress, err := resolveEgressInterface(backend)
	if err != nil {
		return backendValue{}, err
	}
	if len(egress.HardwareAddr) != 6 {
		return backendValue{}, fmt.Errorf("egress interface %q has no ethernet mac", egress.Name)
	}

	var destination [6]byte
	copy(destination[:], destinationMAC)
	var source [6]byte
	copy(source[:], egress.HardwareAddr)

	return backendValue{
		IP:      ipv4ToNetworkOrder(ip),
		IfIndex: uint32(egress.Index),
		DstMAC:  destination,
		SrcMAC:  source,
	}, nil
}

func resolveEgressInterface(backend BackendConfig) (*net.Interface, error) {
	var byName *net.Interface
	var byIndex *net.Interface
	var err error

	if backend.IfName != "" {
		byName, err = net.InterfaceByName(backend.IfName)
		if err != nil {
			return nil, fmt.Errorf("lookup egress ifname %q: %w", backend.IfName, err)
		}
	}
	if backend.IfIndex > 0 {
		byIndex, err = net.InterfaceByIndex(backend.IfIndex)
		if err != nil {
			return nil, fmt.Errorf("lookup egress ifindex %d: %w", backend.IfIndex, err)
		}
	}

	if byName != nil && byIndex != nil && byName.Index != byIndex.Index {
		return nil, fmt.Errorf("ifname %q and ifindex %d refer to different interfaces", backend.IfName, backend.IfIndex)
	}
	if byName != nil {
		return byName, nil
	}
	if byIndex != nil {
		return byIndex, nil
	}
	return nil, fmt.Errorf("missing egress interface")
}

func BackendLabels(cfg *Config) []BackendLabel {
	labels := make([]BackendLabel, 0)
	var id uint32
	for _, svc := range cfg.Services {
		for _, backend := range svc.Backends {
			labels = append(labels, BackendLabel{
				ID:      id,
				Service: svc.Name,
				Name:    backend.Name,
				IP:      backend.IP,
			})
			id++
		}
	}
	return labels
}
