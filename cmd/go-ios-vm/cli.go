package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/deploymenttheory/go-ios-vm/internal/acquisition"
	"github.com/deploymenttheory/go-ios-vm/internal/inspection"
	"github.com/deploymenttheory/go-ios-vm/internal/model"
	"github.com/deploymenttheory/go-ios-vm/internal/postgres"
	"github.com/deploymenttheory/go-ios-vm/internal/workflow"
)

var (
	errFormatMustBeTextOrJson = errors.New("format must be text or json")
	errInvalidCacheBudget     = errors.New("invalid cache budget")
	errRunOrTargetIsRequired  = errors.New("run or target is required")
	errUnpublishedComparison  = errors.New("both runs must have published results")
	errCandidateSizeMismatch  = errors.New("historical locator size differs from Apple CDN")
)

type partialError struct{}

func (*partialError) Error() string { return "inspection has incomplete coverage" }

type config struct {
	DatabaseURL, Format string
	Cache               acquisition.Cache
	EvidenceLimit       int64
}
type envelope struct {
	SchemaVersion int    `json:"schema_version"`
	State         string `json:"state"`
	Results       any    `json:"results"`
}

func newCommand(stdout, stderr io.Writer) *cobra.Command {
	v := viper.New()
	v.SetEnvPrefix("GOIOSVM")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()
	var cfg config
	var configFile string
	root := &cobra.Command{
		Use:           "go-ios-vm",
		Short:         "Inspect Apple firmware with durable, source-addressable evidence",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = ".cache"
	}
	flags := root.PersistentFlags()
	flags.StringVar(&configFile, "config", "", "Viper configuration file")
	flags.String("database-url", "", "PostgreSQL URL (prefer GOIOSVM_DATABASE_URL)")
	flags.String("format", "text", "Output format: text or json")
	flags.String(
		"cache-dir",
		filepath.Join(cacheDir, "go-ios-vm", "ipsw"),
		"Managed disposable IPSW cache",
	)
	flags.Int64(
		"evidence-limit",
		2<<30,
		"Maximum distinct supporting bytes retained in PostgreSQL (excludes SQL/index overhead)",
	)
	flags.Int64("cache-limit", 8<<30, "Maximum managed IPSW bytes")
	flags.Int64("disk-reserve", 2<<30, "Free disk bytes to preserve after downloads")
	if err := v.BindPFlags(flags); err != nil {
		panic(err)
	}
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if configFile != "" {
			v.SetConfigFile(configFile)
			if err := v.ReadInConfig(); err != nil {
				return err
			}
		}
		cfg = config{
			DatabaseURL:   v.GetString("database-url"),
			Format:        v.GetString("format"),
			EvidenceLimit: v.GetInt64("evidence-limit"),
			Cache: acquisition.Cache{
				Dir:     v.GetString("cache-dir"),
				Limit:   v.GetInt64("cache-limit"),
				Reserve: v.GetInt64("disk-reserve"),
			},
		}
		if cfg.Format != "text" && cfg.Format != "json" {
			return errFormatMustBeTextOrJson
		}
		if cfg.Cache.Limit <= 0 || cfg.Cache.Reserve < 0 || cfg.EvidenceLimit <= 0 {
			return errInvalidCacheBudget
		}
		return nil
	}
	emit := func(state string, result any) error {
		value := envelope{model.SchemaVersion, state, result}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if cfg.Format == "text" {
			fmt.Fprintf(stdout, "%s\n", strings.ToUpper(state))
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		return enc.Encode(value)
	}
	withDB := func(ctx context.Context, fn func(*postgres.Store) error) error {
		s, err := postgres.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer s.Close()
		s.EvidenceLimit = cfg.EvidenceLimit
		return fn(s)
	}
	command := func(parent *cobra.Command, use, short string, fn func(*cobra.Command, []string) error) *cobra.Command {
		c := &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: fn}
		parent.AddCommand(c)
		return c
	}
	db := &cobra.Command{Use: "db", Short: "Evidence database"}
	root.AddCommand(db)
	command(
		db,
		"migrate",
		"Apply embedded evidence schema migrations",
		func(c *cobra.Command, _ []string) error {
			return withDB(c.Context(), func(s *postgres.Store) error {
				if err := s.Migrate(c.Context()); err != nil {
					return err
				}
				return emit("complete", map[string]string{"database": "schema current"})
			})
		},
	)
	firmware := &cobra.Command{
		Use:   "firmware",
		Short: "Discover and acquire Apple restore firmware",
	}
	root.AddCommand(firmware)
	var listProduct, listSearch string
	list := command(firmware, "list", "List and search unverified restore IPSW locators", func(c *cobra.Command, _ []string) error {
		if listProduct == "" {
			return errors.New("firmware list requires --product")
		}
		return withDB(c.Context(), func(s *postgres.Store) error {
			fmt.Fprintln(stderr, "Reading restore catalogs…")
			responses := acquisition.NewClient().List(c.Context(), listProduct)
			for _, response := range responses {
				if err := s.Acquisition(c.Context(), response.Source,
					map[string]any{"selection": acquisition.Request{Product: listProduct}, "search": listSearch, "diagnostic": response.Error},
					response.Status, response.Data); err != nil {
					return err
				}
			}
			if err := c.Context().Err(); err != nil {
				return err
			}
			listing := acquisition.BuildListing(responses, listProduct, listSearch)
			state := listingState(listing)
			if cfg.Format == "text" {
				printFirmwareListing(stdout, state, listing)
			} else if err := emit(state, listing); err != nil {
				return err
			}
			if state == "partial" {
				return &partialError{}
			}
			return nil
		})
	})
	list.Flags().StringVar(&listProduct, "product", "", "Exact product identifier, e.g. iPhone12,1")
	list.Flags().StringVar(&listSearch, "search", "", "Case-insensitive version or build substring")
	for _, mode := range []string{"discover", "probe", "fetch"} {
		var req acquisition.Request
		short := map[string]string{
			"discover": "Resolve restore image candidates and retain source responses",
			"probe":    "Check an exact restore IPSW BuildManifest using bounded byte ranges",
			"fetch":    "Download and verify one exact restore IPSW",
		}[mode]
		c := command(firmware, mode, short, func(c *cobra.Command, _ []string) error {
			return withDB(c.Context(), func(s *postgres.Store) error {
				client := acquisition.NewClient()
				fmt.Fprintln(stderr, "Reading restore catalogs…")
				responses := client.Discover(c.Context(), req)
				for _, response := range responses {
					if err := s.Acquisition(
						c.Context(),
						response.Source,
						map[string]any{"selection": req, "diagnostic": response.Error},
						response.Status,
						response.Data,
					); err != nil {
						return err
					}
				}
				if err := c.Context().Err(); err != nil {
					return err
				}
				if mode == "discover" {
					state := "complete"
					for _, response := range responses {
						if response.Status == "failed" {
							state = "partial"
						}
					}
					if err := emit(state, responses); err != nil {
						return err
					}
					if state == "partial" {
						return &partialError{}
					}
					return nil
				}
				selected, err := acquisition.Select(responses, req)
				if err != nil {
					return err
				}
				fmt.Fprintln(
					stderr,
					"Probing BuildManifest for",
					selected.Product,
					selected.Version,
					selected.Build,
				)
				remote, err := client.OpenRange(c.Context(), selected.URL)
				if err != nil {
					return err
				}
				if selected.Size > 0 && selected.Size != remote.Size() {
					return fmt.Errorf(
						"%w: index %d, Apple CDN %d",
						errCandidateSizeMismatch,
						selected.Size,
						remote.Size(),
					)
				}
				probe, err := inspection.ProbeManifest(
					c.Context(),
					remote,
					remote.Size(),
					selected.Product,
					selected.Version,
					selected.Build,
				)
				if err != nil {
					return err
				}
				info := remote.Info()
				selected.Size = info.Size
				selected.ETag = info.ETag
				if err := s.Acquisition(
					c.Context(),
					selected.URL,
					map[string]any{
						"candidate":       selected,
						"range":           info,
						"manifest_sha256": probe.ManifestSHA256,
					},
					"manifest_probe",
					probe.RawBuildManifest,
				); err != nil {
					return err
				}
				if mode == "probe" {
					return emit("complete", map[string]any{
						"candidate": selected, "range": info, "manifest": probe,
						"full_archive_verified": false,
					})
				}
				fmt.Fprintln(stderr, "Downloading and verifying full IPSW")
				a, err := client.Fetch(
					c.Context(), selected, cfg.Cache,
					func(ctx context.Context, path string, selected acquisition.Candidate) error {
						image, err := inspection.Open(ctx, path)
						if err != nil {
							return err
						}
						defer image.Close()
						return image.Verify(selected.Product, selected.Version, selected.Build)
					},
				)
				if err != nil {
					return err
				}
				receipt, _ := json.Marshal(
					map[string]any{"candidate": selected, "artifact": a, "identity_verified": true},
				)
				if err = s.Acquisition(
					c.Context(),
					selected.URL,
					req,
					"verified",
					receipt,
				); err != nil {
					return err
				}
				return emit("complete", a)
			})
		})
		c.Flags().
			StringVar(&req.Product, "product", "", "Exact product identifier, e.g. iPhone12,1")
		c.Flags().StringVar(&req.Version, "version", "", "Exact version, e.g. 14.0")
		c.Flags().StringVar(&req.Build, "build", "", "Exact build")
	}

	target := &cobra.Command{Use: "target", Short: "Build identities from an IPSW"}
	root.AddCommand(target)
	var targetImage string
	c := command(
		target,
		"list",
		"Read and register the actual BuildManifest identities",
		func(c *cobra.Command, _ []string) error {
			return withDB(c.Context(), func(s *postgres.Store) error {
				release, err := cfg.Cache.Lease(c.Context(), false)
				if err != nil {
					return err
				}
				defer release()
				a, err := inspection.Open(c.Context(), targetImage)
				if err != nil {
					return err
				}
				defer a.Close()
				if err = s.Register(c.Context(), a.Artifact, a.Targets); err != nil {
					return err
				}
				return emit("complete", a.Targets)
			})
		},
	)
	c.Flags().StringVar(&targetImage, "image", "", "IPSW path")
	_ = c.MarkFlagRequired("image")
	var inspectImage string
	var identity int
	c = command(
		root,
		"inspect",
		"Inspect a selected identity and publish evidence atomically",
		func(c *cobra.Command, _ []string) error {
			return withDB(c.Context(), func(s *postgres.Store) error {
				release, err := cfg.Cache.Lease(c.Context(), false)
				if err != nil {
					return err
				}
				defer release()
				fmt.Fprintln(stderr, "Hashing IPSW and reading BuildManifest…")
				a, err := inspection.Open(c.Context(), inspectImage)
				if err != nil {
					return err
				}
				defer a.Close()
				run, err := workflow.Inspect(
					c.Context(),
					s,
					a.Artifact,
					a.Targets,
					identity,
					func(ctx context.Context) ([]model.Observation, []model.Coverage, error) {
						r, err := a.Inspect(ctx, identity)
						return r.Observations, r.Coverage, err
					},
				)
				if err != nil {
					return fmt.Errorf("run %s: %w", run.ID, err)
				}
				if err = emit(run.State, run); err != nil {
					return err
				}
				if run.State == "partial" {
					return &partialError{}
				}
				return nil
			})
		},
	)
	c.Flags().StringVar(&inspectImage, "image", "", "IPSW path")
	c.Flags().IntVar(&identity, "identity", -1, "Explicit BuildManifest identity index")
	_ = c.MarkFlagRequired("image")
	_ = c.MarkFlagRequired("identity")
	finding := &cobra.Command{Use: "finding", Short: "Query retained observations and evidence"}
	root.AddCommand(finding)
	var runID, targetID string
	c = command(
		finding,
		"list",
		"List findings for a run or target",
		func(c *cobra.Command, _ []string) error {
			if runID == "" && targetID == "" {
				return errRunOrTargetIsRequired
			}
			return withDB(c.Context(), func(s *postgres.Store) error {
				f, err := s.Findings(c.Context(), targetID, runID)
				if err != nil {
					return err
				}
				return emit("complete", f)
			})
		},
	)
	c.Flags().StringVar(&runID, "run", "", "Run ID")
	c.Flags().StringVar(&targetID, "target", "", "Target ID")
	var evidenceID string
	c = command(
		finding,
		"show",
		"Retrieve integrity-checked supporting bytes from PostgreSQL",
		func(c *cobra.Command, _ []string) error {
			return withDB(c.Context(), func(s *postgres.Store) error {
				b, err := s.Evidence(c.Context(), evidenceID)
				if err != nil {
					return err
				}
				return emit(
					"complete",
					map[string]any{
						"evidence_id": evidenceID,
						"size":        len(b),
						"encoding":    "base64",
						"data":        b,
					},
				)
			})
		},
	)
	c.Flags().StringVar(&evidenceID, "evidence", "", "Evidence SHA-256")
	_ = c.MarkFlagRequired("evidence")
	var left, right string
	c = command(
		root,
		"compare",
		"Compare exact subject/property observations from two retained runs",
		func(c *cobra.Command, _ []string) error {
			return withDB(c.Context(), func(s *postgres.Store) error {
				lr, err := s.Run(c.Context(), left)
				if err != nil {
					return err
				}
				rr, err := s.Run(c.Context(), right)
				if err != nil {
					return err
				}
				if (lr.State != "complete" && lr.State != "partial") ||
					(rr.State != "complete" && rr.State != "partial") {
					return errUnpublishedComparison
				}
				l, err := s.Findings(c.Context(), "", left)
				if err != nil {
					return err
				}
				r, err := s.Findings(c.Context(), "", right)
				if err != nil {
					return err
				}
				return emit(
					"complete",
					map[string]any{
						"left":     lr,
						"right":    rr,
						"matching": "exact subject/property; cross-hardware equivalence is not inferred",
						"changes":  inspection.Compare(l, r),
					},
				)
			})
		},
	)
	c.Flags().StringVar(&left, "left", "", "Left run ID")
	c.Flags().StringVar(&right, "right", "", "Right run ID")
	_ = c.MarkFlagRequired("left")
	_ = c.MarkFlagRequired("right")
	cache := &cobra.Command{
		Use:   "cache",
		Short: "Manage disposable IPSWs independently of retained database evidence",
	}
	root.AddCommand(cache)
	command(
		cache,
		"status",
		"List managed cache entries",
		func(c *cobra.Command, _ []string) error {
			items, err := cfg.Cache.Entries()
			if err != nil {
				return err
			}
			return emit("complete", items)
		},
	)
	var apply bool
	c = command(
		cache,
		"prune",
		"Preview inactive managed image deletion; --apply deletes them",
		func(c *cobra.Command, _ []string) error {
			items, err := cfg.Cache.Prune(c.Context(), apply)
			if err != nil {
				return err
			}
			return emit("complete", map[string]any{"applied": apply, "entries": items})
		},
	)
	c.Flags().
		BoolVar(&apply, "apply", false, "Delete listed managed images after acquiring exclusive cache lease")
	return root
}

func printFirmwareListing(w io.Writer, state string, listing acquisition.Listing) {
	fmt.Fprintf(w, "%s: restore IPSW catalog locators for %s\n", strings.ToUpper(state), listing.Product)
	if listing.Query != "" {
		fmt.Fprintf(w, "Search: %q\n", listing.Query)
	}
	for _, source := range listing.Sources {
		if source.Status == "failed" {
			fmt.Fprintf(w, "Source failed: %s (%s)\n", source.Source, source.Error)
		}
	}
	if len(listing.Restores) == 0 {
		fmt.Fprintln(w, "No matching restore IPSW locators found in the sources that responded.")
		return
	}
	for _, row := range listing.Restores {
		fmt.Fprintf(w, "%s %s  %s", row.Version, row.Build, row.Status)
		if row.Size > 0 {
			fmt.Fprintf(w, "  %d bytes", row.Size)
		}
		if row.SHA1 != "" {
			fmt.Fprintf(w, "  SHA-1 %s", row.SHA1)
		}
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  source: %s\n  URL: %s\n", strings.Join(row.Sources, ", "), row.URL)
		if row.NextCommand != "" {
			fmt.Fprintf(w, "  next: go-ios-vm %s\n", row.NextCommand)
		}
	}
	fmt.Fprintln(w, "Catalog locators are unverified; fetch checks the full archive and BuildManifest.")
}

func listingState(listing acquisition.Listing) string {
	for _, source := range listing.Sources {
		if source.Status == "failed" {
			return "partial"
		}
	}
	return "complete"
}
