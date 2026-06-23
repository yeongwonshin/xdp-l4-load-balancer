package main

import (
	"github.com/cilium/ebpf"
	"github.com/prometheus/client_golang/prometheus"
)

type BackendCollector struct {
	statsMap *ebpf.Map
	labels   []BackendLabel

	packets *prometheus.Desc
	bytes   *prometheus.Desc
	flows   *prometheus.Desc
}

func NewBackendCollector(statsMap *ebpf.Map, cfg *Config) *BackendCollector {
	labelNames := []string{"backend_id", "service", "backend", "ip"}
	return &BackendCollector{
		statsMap: statsMap,
		labels:   BackendLabels(cfg),
		packets: prometheus.NewDesc("xdp_l4lb_backend_packets_total",
			"Packets redirected to backend by the XDP datapath.", labelNames, nil),
		bytes: prometheus.NewDesc("xdp_l4lb_backend_bytes_total",
			"Bytes redirected to backend by the XDP datapath.", labelNames, nil),
		flows: prometheus.NewDesc("xdp_l4lb_backend_flows_total",
			"New 5-tuple flows assigned to backend.", labelNames, nil),
	}
}

func (c *BackendCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.packets
	ch <- c.bytes
	ch <- c.flows
}

func (c *BackendCollector) Collect(ch chan<- prometheus.Metric) {
	for _, label := range c.labels {
		var st backendStats
		if err := c.statsMap.Lookup(label.ID, &st); err != nil {
			continue
		}
		values := []string{uint32ToString(label.ID), label.Service, label.Name, label.IP}
		ch <- prometheus.MustNewConstMetric(c.packets, prometheus.CounterValue, float64(st.Packets), values...)
		ch <- prometheus.MustNewConstMetric(c.bytes, prometheus.CounterValue, float64(st.Bytes), values...)
		ch <- prometheus.MustNewConstMetric(c.flows, prometheus.CounterValue, float64(st.Flows), values...)
	}
}

func uint32ToString(v uint32) string {
	if v == 0 {
		return "0"
	}
	buf := [10]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
