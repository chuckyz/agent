package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"

	"github.com/grafana/agent/web/api"
	"github.com/grafana/agent/web/ui"
	"golang.org/x/exp/maps"

	"github.com/fatih/color"
	"github.com/go-kit/log/level"
	"github.com/gorilla/mux"
	"github.com/grafana/agent/pkg/flow"
	"github.com/grafana/agent/pkg/flow/logging"
	"github.com/grafana/agent/pkg/river/diag"
	"github.com/grafana/agent/pkg/usagestats"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"go.uber.org/atomic"

	// Install Components
	_ "github.com/grafana/agent/component/all"
)

func runCommand() *cobra.Command {
	r := &flowRun{
		httpListenAddr:   "127.0.0.1:12345",
		storagePath:      "data-agent/",
		uiPrefix:         "/",
		disableReporting: false,
	}

	cmd := &cobra.Command{
		Use:   "run [flags] file",
		Short: "Run Grafana Agent Flow",
		Long: `The run subcommand runs Grafana Agent Flow in the foreground until an interrupt
is received.

run must be provided an argument pointing at the River file to use. If the
River file wasn't specified, can't be loaded, or contains errors, run will exit
immediately.

run starts an HTTP server which can be used to debug Grafana Agent Flow or
force it to reload (by sending a GET or POST request to /-/reload). The listen
address can be changed through the --server.http.listen-addr flag.

By default, the HTTP server exposes a debugging UI at /. The path of the
debugging UI can be changed by providing a different value to
--server.http.ui-path-prefix.

Additionally, the HTTP server exposes the following debug endpoints:

  /debug/pprof   Go performance profiling tools

If reloading the config file fails, Grafana Agent Flow will continue running in
its last valid state. Components which failed may be be listed as unhealthy,
depending on the nature of the reload error.
`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,

		RunE: func(cmd *cobra.Command, args []string) error {
			return r.Run(args[0])
		},
	}

	cmd.Flags().
		StringVar(&r.httpListenAddr, "server.http.listen-addr", r.httpListenAddr, "address to listen for HTTP traffic on")
	cmd.Flags().StringVar(&r.storagePath, "storage.path", r.storagePath, "Base directory where components can store data")
	cmd.Flags().StringVar(&r.uiPrefix, "server.http.ui-path-prefix", r.uiPrefix, "Prefix to serve the HTTP UI at")
	cmd.Flags().
		BoolVar(&r.disableReporting, "disable-reporting", r.disableReporting, "Disable reporting of enabled components to Grafana.")
	return cmd
}

type flowRun struct {
	httpListenAddr   string
	storagePath      string
	uiPrefix         string
	disableReporting bool
}

func (fr *flowRun) Run(configFile string) error {
	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancel := interruptContext()
	defer cancel()

	if configFile == "" {
		return fmt.Errorf("file argument not provided")
	}

	l, err := logging.New(os.Stderr, logging.DefaultOptions)
	if err != nil {
		return fmt.Errorf("building logger: %w", err)
	}

	f := flow.New(flow.Options{
		Logger:         l,
		DataPath:       fr.storagePath,
		Reg:            prometheus.DefaultRegisterer,
		HTTPListenAddr: fr.httpListenAddr,
	})

	reload := func() error {
		flowCfg, err := loadFlowFile(configFile)
		if err != nil {
			return fmt.Errorf("reading config file %q: %w", configFile, err)
		}
		if err := f.LoadFile(flowCfg); err != nil {
			return fmt.Errorf("error during the initial gragent load: %w", err)
		}
		return nil
	}

	if err := reload(); err != nil {
		var diags diag.Diagnostics
		if errors.As(err, &diags) {
			bb, _ := os.ReadFile(configFile)

			p := diag.NewPrinter(diag.PrinterConfig{
				Color:              !color.NoColor,
				ContextLinesBefore: 1,
				ContextLinesAfter:  1,
			})
			_ = p.Fprint(os.Stderr, map[string][]byte{configFile: bb}, diags)

			// Print newline after the diagnostics.
			fmt.Println()

			return fmt.Errorf("could not perform the initial load successfully")
		}

		// Exit if the initial load files
		return err
	}

	// HTTP server
	{
		lis, err := net.Listen("tcp", fr.httpListenAddr)
		if err != nil {
			return fmt.Errorf("failed to listen on %s: %w", fr.httpListenAddr, err)
		}

		r := mux.NewRouter()

		r.Handle("/metrics", promhttp.Handler())
		r.PathPrefix("/debug/pprof").Handler(http.DefaultServeMux)
		r.PathPrefix("/component/{id}/").Handler(f.ComponentHandler())

		ready := atomic.NewBool(true)
		r.HandleFunc("/-/ready", func(w http.ResponseWriter, r *http.Request) {
			if ready.Load() {
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, "Agent is Ready.\n")
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprint(w, "Config failed to load.\n")
			}
		})

		r.HandleFunc("/-/reload", func(w http.ResponseWriter, _ *http.Request) {
			err := reload()
			ready.Store(err == nil)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			fmt.Fprintln(w, "config reloaded")
		}).Methods(http.MethodGet, http.MethodPost)

		// Register Routes must be the last
		fa := api.NewFlowAPI(f, r)
		fa.RegisterRoutes(fr.uiPrefix, r)

		// NOTE(rfratto): keep this at the bottom of all other routes, otherwise it
		// will take precedence over anything else mapped in uiPrefix.
		ui.RegisterRoutes(fr.uiPrefix, r)

		srv := &http.Server{Handler: r}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancel()

			level.Info(l).Log("msg", "now listening for http traffic", "addr", fr.httpListenAddr)
			if err := srv.Serve(lis); err != nil {
				level.Info(l).Log("msg", "http server closed", "err", err)
			}
		}()

		defer func() { _ = srv.Shutdown(ctx) }()
	}

	// Report usage of enabled components
	if !fr.disableReporting {
		reporter, err := usagestats.NewReporter(l)
		if err != nil {
			return fmt.Errorf("failed to create reporter: %w", err)
		}
		go func() {
			err := reporter.Start(ctx, getEnabledComponentsFunc(f))
			if err != nil {
				level.Error(l).Log("msg", "failed to start reporter", "err", err)
			}
		}()
	}

	<-ctx.Done()
	return f.Close()
}

// getEnabledComponentsFunc returns a function that gets the current enabled components
func getEnabledComponentsFunc(f *flow.Flow) func() map[string]interface{} {
	return func() map[string]interface{} {
		infos := f.ComponentInfos()
		components := map[string]struct{}{}
		for _, info := range infos {
			components[info.Name] = struct{}{}
		}
		return map[string]interface{}{"enabled-components": maps.Keys(components)}
	}
}

func loadFlowFile(filename string) (*flow.File, error) {
	bb, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return flow.ReadFile(filename, bb)
}

func interruptContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer cancel()
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		select {
		case <-sig:
		case <-ctx.Done():
		}
		signal.Stop(sig)

		fmt.Fprintln(os.Stderr, "interrupt received")
	}()

	return ctx, cancel
}
