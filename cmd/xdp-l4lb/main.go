package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -D__TARGET_ARCH_x86" bpf ../../bpf/xdp_l4lb.bpf.c -- -I../../bpf -I/usr/include

func main() {
	ifaceName := flag.String("iface", "", "interface to attach XDP program to")
	cfgPath := flag.String("config", "configs/example.yaml", "load balancer config path")
	metricsAddr := flag.String("metrics", ":2112", "Prometheus metrics listen address")
	xdpMode := flag.String("xdp-mode", "generic", "XDP attach mode: generic, driver, hw")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if *ifaceName == "" {
		logger.Error("-iface is required")
		os.Exit(2)
	}

	cfg, err := LoadConfig(*cfgPath)
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	iface, err := net.InterfaceByName(*ifaceName)
	if err != nil {
		logger.Error("lookup interface", "iface", *ifaceName, "error", err)
		os.Exit(1)
	}

	if len(iface.HardwareAddr) != 6 {
		logger.Error("interface has no ethernet MAC", "iface", iface.Name)
		os.Exit(1)
	}

	if err := rlimit.RemoveMemlock(); err != nil {
		logger.Error("remove memlock limit", "error", err)
		os.Exit(1)
	}

	var objs bpfObjects
	if err := loadBpfObjects(&objs, &ebpf.CollectionOptions{}); err != nil {
		logger.Error("load eBPF objects", "error", err)
		os.Exit(1)
	}
	defer objs.Close()

	if err := ProgramMaps(&objs, cfg, iface.HardwareAddr); err != nil {
		logger.Error("program maps", "error", err)
		os.Exit(1)
	}

	flags, err := parseXDPMode(*xdpMode)
	if err != nil {
		logger.Error("parse xdp mode", "error", err)
		os.Exit(2)
	}

	xdpLink, err := link.AttachXDP(link.XDPOptions{
		Program:   objs.XdpL4lb,
		Interface: iface.Index,
		Flags:     flags,
	})
	if err != nil {
		logger.Error("attach XDP", "iface", iface.Name, "mode", *xdpMode, "error", err)
		os.Exit(1)
	}
	defer xdpLink.Close()

	registry := prometheus.NewRegistry()
	registry.MustRegister(NewBackendCollector(objs.BackendStats, cfg))

	srv := &http.Server{
		Addr:    *metricsAddr,
		Handler: promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("metrics server started", "addr", *metricsAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("XDP L4 load balancer running", "iface", iface.Name, "mode", *xdpMode, "services", len(cfg.Services))

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
	case err := <-errCh:
		logger.Error("metrics server failed", "error", err)
	}

	_ = srv.Shutdown(context.Background())
}

func parseXDPMode(mode string) (link.XDPAttachFlags, error) {
	switch mode {
	case "generic":
		return link.XDPGenericMode, nil
	case "driver":
		return link.XDPDriverMode, nil
	case "hw":
		return link.XDPOffloadMode, nil
	default:
		return 0, fmt.Errorf("unknown XDP mode %q", mode)
	}
}
