package main

import "encoding/binary"

const (
	protoTCP  = 6
	protoUDP  = 17
	lbFlagDSR = 0x1

	maxServices = 4096
	maxBackends = 65536
)

type serviceKey struct {
	VIP   uint32
	Port  uint16
	Proto uint8
	Pad   uint8
}

type serviceValue struct {
	BackendStart uint32
	BackendCount uint32
	Flags        uint32
	Reserved     uint32
}

type backendValue struct {
	IP      uint32
	IfIndex uint32
	DstMAC  [6]byte
	SrcMAC  [6]byte
}

type backendStats struct {
	Packets uint64
	Bytes   uint64
	Flows   uint64
}

func ipv4ToNetworkOrder(ip4 []byte) uint32 {
	return binary.NativeEndian.Uint32(ip4)
}

func portToNetworkOrder(port uint16) uint16 {
	var wire [2]byte
	binary.BigEndian.PutUint16(wire[:], port)
	return binary.NativeEndian.Uint16(wire[:])
}
