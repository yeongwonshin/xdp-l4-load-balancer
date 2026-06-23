package main

import "encoding/binary"

const (
	protoTCP  = 6
	protoUDP  = 17
	lbFlagDSR = 0x1
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
	MAC     [6]byte
	Pad     [2]byte
}

type backendStats struct {
	Packets uint64
	Bytes   uint64
	Flows   uint64
}

type lbConfig struct {
	SrcMAC        [6]byte
	Pad           [2]byte
	DefaultAction uint32
}

func ipv4ToU32NetworkOrder(ip4 []byte) uint32 {
	return binary.BigEndian.Uint32(ip4)
}

func portToNetworkOrder(port uint16) uint16 {
	return (port<<8)&0xff00 | port>>8
}
