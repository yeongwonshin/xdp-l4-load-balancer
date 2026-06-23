package main

import (
	"fmt"
	"net"
	"strings"
)

type BackendLabel struct {
	ID      uint32
	Service string
	Name    string
	IP      string
}

func ProgramMaps(objs *bpfObjects, cfg *Config, lbMAC net.HardwareAddr) error {
	var localMAC [6]byte
	copy(localMAC[:], lbMAC)

	// XDP_PASS is 2; kept configurable for future drop/pass policy support.
	if err := objs.LbConfig.Update(uint32(0), lbConfig{SrcMAC: localMAC, DefaultAction: 2}, 0); err != nil {
		return fmt.Errorf("update lb_config: %w", err)
	}

	var backendID uint32
	for _, svc := range cfg.Services {
		start := backendID
		for _, be := range svc.Backends {
			val, err := backendToMapValue(be)
			if err != nil {
				return err
			}
			if err := objs.Backends.Update(backendID, val, 0); err != nil {
				return fmt.Errorf("update backend %d: %w", backendID, err)
			}
			if err := objs.BackendStats.Update(backendID, backendStats{}, 0); err != nil {
				return fmt.Errorf("clear backend_stats %d: %w", backendID, err)
			}
			backendID++
		}

		svcKey, err := serviceToMapKey(svc)
		if err != nil {
			return err
		}
		svcVal := serviceValue{
			BackendStart: start,
			BackendCount: uint32(len(svc.Backends)),
			Flags:        lbFlagDSR,
		}
		if err := objs.Services.Update(svcKey, svcVal, 0); err != nil {
			return fmt.Errorf("update service %s: %w", svc.Name, err)
		}
	}
	return nil
}

func serviceToMapKey(svc ServiceConfig) (serviceKey, error) {
	ip := net.ParseIP(svc.VIP).To4()
	if ip == nil {
		return serviceKey{}, fmt.Errorf("invalid VIP %q", svc.VIP)
	}

	proto := uint8(protoTCP)
	if strings.ToLower(svc.Protocol) == "udp" {
		proto = protoUDP
	}

	return serviceKey{
		VIP:   ipv4ToU32NetworkOrder(ip),
		Port:  portToNetworkOrder(svc.Port),
		Proto: proto,
	}, nil
}

func backendToMapValue(be BackendConfig) (backendValue, error) {
	ip := net.ParseIP(be.IP).To4()
	if ip == nil {
		return backendValue{}, fmt.Errorf("invalid backend IP %q", be.IP)
	}

	mac, err := net.ParseMAC(be.MAC)
	if err != nil {
		return backendValue{}, fmt.Errorf("parse backend MAC %q: %w", be.MAC, err)
	}

	ifindex := be.IfIndex
	if ifindex == 0 {
		iface, err := net.InterfaceByName(be.IfName)
		if err != nil {
			return backendValue{}, fmt.Errorf("lookup backend egress ifname %q: %w", be.IfName, err)
		}
		ifindex = iface.Index
	}

	var macBytes [6]byte
	copy(macBytes[:], mac)

	return backendValue{
		IP:      ipv4ToU32NetworkOrder(ip),
		IfIndex: uint32(ifindex),
		MAC:     macBytes,
	}, nil
}

func BackendLabels(cfg *Config) []BackendLabel {
	labels := make([]BackendLabel, 0)
	var id uint32
	for _, svc := range cfg.Services {
		for _, be := range svc.Backends {
			name := be.Name
			if name == "" {
				name = be.IP
			}
			labels = append(labels, BackendLabel{ID: id, Service: svc.Name, Name: name, IP: be.IP})
			id++
		}
	}
	return labels
}
