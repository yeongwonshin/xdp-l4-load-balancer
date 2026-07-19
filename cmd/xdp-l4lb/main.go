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
	"strings"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//go:generate ../../scripts/generate-bpf.sh

type options struct {
	ifaceName   string
	configPath  string
	metricsAddr string
	xdpMode     string
	checkConfig bool
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(parseOptions(), logger); err != nil {
		logger.Error("xdp l4 load balancer stopped", "error", err)
		os.Exit(1)
	}
}

func parseOptions() options {
	var opts options
	flag.StringVar(&opts.ifaceName, "iface", "", "interface to attach the xdp program to")
	flag.StringVar(&opts.configPath, "config", "configs/example.yaml", "load balancer config path")
	flag.StringVar(&opts.metricsAddr, "metrics", ":2112", "prometheus and health endpoint listen address")
	flag.StringVar(&opts.xdpMode, "xdp-mode", "generic", "xdp attach mode: generic, driver, hw")
	flag.BoolVar(&opts.checkConfig, "check-config", false, "validate configuration and exit")
	flag.Parse()
	return opts
}

func run(opts options, logger *slog.Logger) error {
	cfg, err := LoadConfig(opts.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	backendCount := 0
	for _, service := range cfg.Services {
		backendCount += len(service.Backends)
	}
	if opts.checkConfig {
		logger.Info("configuration is valid", "services", len(cfg.Services), "backends", backendCount)
		return nil
	}
	ifaceName := strings.TrimSpace(opts.ifaceName)
	if ifaceName == "" {
		return fmt.Errorf("-iface is required")
	}

	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return fmt.Errorf("lookup interface %q: %w", ifaceName, err)
	}
	if len(iface.HardwareAddr) != 6 {
		return fmt.Errorf("interface %q has no ethernet mac", iface.Name)
	}

	attachFlags, err := parseXDPMode(opts.xdpMode)
	if err != nil {
		return err
	}

	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("remove memlock limit: %w", err)
	}

	var objects bpfObjects
	if err := loadBpfObjects(&objects, &ebpf.CollectionOptions{}); err != nil {
		return fmt.Errorf("load ebpf objects: %w", err)
	}
	defer objects.Close()

	if err := ProgramMaps(&objects, cfg); err != nil {
		return fmt.Errorf("program maps: %w", err)
	}

	xdpLink, err := link.AttachXDP(link.XDPOptions{
		Program:   objects.XdpL4lb,
		Interface: iface.Index,
		Flags:     attachFlags,
	})
	if err != nil {
		return fmt.Errorf("attach xdp to interface %q in mode %q: %w", iface.Name, opts.xdpMode, err)
	}
	defer xdpLink.Close()

	collector, err := NewLoadBalancerCollector(objects.BackendStats, objects.DatapathStats, cfg)
	if err != nil {
		return fmt.Errorf("create metrics collector: %w", err)
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
		ErrorHandling:     promhttp.ContinueOnError,
	}))
	mux.HandleFunc("/healthz", plainTextStatus(http.StatusOK, "ok\n"))
	mux.HandleFunc("/readyz", plainTextStatus(http.StatusOK, "ready\n"))

	listener, err := net.Listen("tcp", opts.metricsAddr)
	if err != nil {
		return fmt.Errorf("listen on metrics address %q: %w", opts.metricsAddr, err)
	}
	defer listener.Close()

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("metrics server started", "addr", listener.Addr().String())
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			serverErrors <- serveErr
		}
	}()

	signalContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info(
		"xdp l4 load balancer running",
		"iface", iface.Name,
		"mode", strings.ToLower(strings.TrimSpace(opts.xdpMode)),
		"services", len(cfg.Services),
		"backends", backendCount,
	)

	var runErr error
	select {
	case <-signalContext.Done():
		logger.Info("shutdown signal received")
	case serveErr := <-serverErrors:
		runErr = fmt.Errorf("metrics server failed: %w", serveErr)
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil && runErr == nil {
		runErr = fmt.Errorf("shutdown metrics server: %w", err)
	}
	return runErr
}

func plainTextStatus(status int, body string) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("content-type", "text/plain; charset=utf-8")
		writer.WriteHeader(status)
		_, _ = writer.Write([]byte(body))
	}
}

func parseXDPMode(mode string) (link.XDPAttachFlags, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "generic":
		return link.XDPGenericMode, nil
	case "driver":
		return link.XDPDriverMode, nil
	case "hw":
		return link.XDPOffloadMode, nil
	default:
		return 0, fmt.Errorf("unknown xdp mode %q", mode)
	}
}
