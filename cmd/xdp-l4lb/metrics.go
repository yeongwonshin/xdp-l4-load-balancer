package main

import (
	"fmt"
	"strconv"

	"github.com/cilium/ebpf"
	"github.com/prometheus/client_golang/prometheus"
)

var datapathEvents = []string{
	"packets_seen",
	"non_ipv4",
	"malformed",
	"fragmented",
	"unsupported_l4",
	"service_miss",
	"flow_insert_failure",
	"backend_miss",
	"redirect_requested",
}

type LoadBalancerCollector struct {
	backendStatsMap  *ebpf.Map
	datapathStatsMap *ebpf.Map
	labels           []BackendLabel
	possibleCPUs     int

	packets        *prometheus.Desc
	bytes          *prometheus.Desc
	flows          *prometheus.Desc
	datapathEvents *prometheus.Desc
}

func NewLoadBalancerCollector(backendStatsMap, datapathStatsMap *ebpf.Map, cfg *Config) (*LoadBalancerCollector, error) {
	possibleCPUs, err := ebpf.PossibleCPU()
	if err != nil {
		return nil, fmt.Errorf("detect possible cpus: %w", err)
	}

	backendLabelNames := []string{"backend_id", "service", "backend", "ip"}
	return &LoadBalancerCollector{
		backendStatsMap:  backendStatsMap,
		datapathStatsMap: datapathStatsMap,
		labels:           BackendLabels(cfg),
		possibleCPUs:     possibleCPUs,
		packets: prometheus.NewDesc(
			"xdp_l4lb_backend_packets_total",
			"Packets selected for redirect to a backend by the XDP datapath.",
			backendLabelNames,
			nil,
		),
		bytes: prometheus.NewDesc(
			"xdp_l4lb_backend_bytes_total",
			"Bytes selected for redirect to a backend by the XDP datapath.",
			backendLabelNames,
			nil,
		),
		flows: prometheus.NewDesc(
			"xdp_l4lb_backend_flows_total",
			"New five tuple flows assigned to a backend.",
			backendLabelNames,
			nil,
		),
		datapathEvents: prometheus.NewDesc(
			"xdp_l4lb_datapath_events_total",
			"Datapath processing events by outcome.",
			[]string{"event"},
			nil,
		),
	}, nil
}

func (c *LoadBalancerCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.packets
	ch <- c.bytes
	ch <- c.flows
	ch <- c.datapathEvents
}

func (c *LoadBalancerCollector) Collect(ch chan<- prometheus.Metric) {
	c.collectBackendStats(ch)
	c.collectDatapathStats(ch)
}

func (c *LoadBalancerCollector) collectBackendStats(ch chan<- prometheus.Metric) {
	for _, label := range c.labels {
		perCPU := make([]backendStats, c.possibleCPUs)
		if err := c.backendStatsMap.Lookup(label.ID, &perCPU); err != nil {
			ch <- prometheus.NewInvalidMetric(c.packets, fmt.Errorf("lookup backend %d stats: %w", label.ID, err))
			continue
		}

		var total backendStats
		for _, stats := range perCPU {
			total.Packets += stats.Packets
			total.Bytes += stats.Bytes
			total.Flows += stats.Flows
		}

		values := []string{strconv.FormatUint(uint64(label.ID), 10), label.Service, label.Name, label.IP}
		ch <- prometheus.MustNewConstMetric(c.packets, prometheus.CounterValue, float64(total.Packets), values...)
		ch <- prometheus.MustNewConstMetric(c.bytes, prometheus.CounterValue, float64(total.Bytes), values...)
		ch <- prometheus.MustNewConstMetric(c.flows, prometheus.CounterValue, float64(total.Flows), values...)
	}
}

func (c *LoadBalancerCollector) collectDatapathStats(ch chan<- prometheus.Metric) {
	for eventID, eventName := range datapathEvents {
		perCPU := make([]uint64, c.possibleCPUs)
		key := uint32(eventID)
		if err := c.datapathStatsMap.Lookup(key, &perCPU); err != nil {
			ch <- prometheus.NewInvalidMetric(c.datapathEvents, fmt.Errorf("lookup datapath event %s: %w", eventName, err))
			continue
		}

		var total uint64
		for _, value := range perCPU {
			total += value
		}
		ch <- prometheus.MustNewConstMetric(c.datapathEvents, prometheus.CounterValue, float64(total), eventName)
	}
}
