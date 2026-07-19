package main

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
)

func TestIPv4ToNetworkOrderPreservesWireBytes(t *testing.T) {
	ip := net.ParseIP("10.10.0.100").To4()
	value := ipv4ToNetworkOrder(ip)

	var encoded [4]byte
	binary.NativeEndian.PutUint32(encoded[:], value)
	if !bytes.Equal(encoded[:], ip) {
		t.Fatalf("map key bytes %v do not match ipv4 wire bytes %v", encoded, ip)
	}
}

func TestPortToNetworkOrderPreservesWireBytes(t *testing.T) {
	value := portToNetworkOrder(8080)

	var encoded [2]byte
	binary.NativeEndian.PutUint16(encoded[:], value)
	want := []byte{0x1f, 0x90}
	if !bytes.Equal(encoded[:], want) {
		t.Fatalf("map key bytes %v do not match port wire bytes %v", encoded, want)
	}
}

func TestServiceToMapKey(t *testing.T) {
	key, err := serviceToMapKey(ServiceConfig{VIP: "192.0.2.10", Port: 53, Protocol: "udp"})
	if err != nil {
		t.Fatalf("serviceToMapKey returned error: %v", err)
	}
	if key.Proto != protoUDP {
		t.Fatalf("unexpected protocol: %d", key.Proto)
	}

	var ipBytes [4]byte
	binary.NativeEndian.PutUint32(ipBytes[:], key.VIP)
	if !bytes.Equal(ipBytes[:], net.ParseIP("192.0.2.10").To4()) {
		t.Fatalf("unexpected vip key bytes: %v", ipBytes)
	}
}
