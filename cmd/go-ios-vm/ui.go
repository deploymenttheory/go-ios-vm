package main

import (
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/deploymenttheory/go-ios-vm/internal/postgres"
	"github.com/deploymenttheory/go-ios-vm/internal/webui"
)

var (
	errUILoopbackRequired    = errors.New("UI listener must use a numeric loopback address and port")
	errUIDatabaseRequired    = errors.New("database URL is required (GOIOSVM_DATABASE_URL)")
	errUIDatabaseUnavailable = errors.New("evidence database is unavailable")
	errUIListenerUnavailable = errors.New("UI listener could not start")
)

func addUICommand(root *cobra.Command, v *viper.Viper, cfg *config, stderr io.Writer) {
	ui := &cobra.Command{Use: "ui", Short: "Local read-only inspection app"}
	root.AddCommand(ui)
	serve := &cobra.Command{
		Use:   "serve",
		Short: "Serve the local inspection app",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if !webui.LoopbackAddress(cfg.UIListen) {
				return classified("invalid_invocation", errUILoopbackRequired)
			}
			if cfg.DatabaseURL == "" {
				return classified("missing_input", errUIDatabaseRequired)
			}
			store, err := postgres.Open(c.Context(), cfg.DatabaseURL)
			if err != nil {
				return classified("storage_failure", errUIDatabaseUnavailable)
			}
			defer store.Close()
			listener, err := net.Listen("tcp", cfg.UIListen)
			if err != nil {
				return classified("operation_failed", errUIListenerUnavailable)
			}
			defer listener.Close()
			fmt.Fprintf(stderr, "Inspection UI: http://%s/\n", listener.Addr().String())
			if err := webui.Serve(c.Context(), listener, webui.NewHandler(store.Ping)); err != nil {
				return classified("operation_failed", errUIListenerUnavailable)
			}
			return nil
		},
	}
	serve.Flags().String("listen", "127.0.0.1:8765", "Numeric loopback address and port for the local UI")
	if err := v.BindPFlag("ui-listen", serve.Flags().Lookup("listen")); err != nil {
		panic(err)
	}
	ui.AddCommand(serve)
}
