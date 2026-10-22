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
	"time"

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
	errFindingShowSelector    = errors.New("specify exactly one of --finding or --evidence")
)

type partialError struct{}

func (*partialError) Error() string { return "inspection has incomplete coverage" }

type config struct {
	DatabaseURL, Format, UIListen string
	Cache                         acquisition.Cache
	EvidenceLimit                 int64
}
type envelope struct {
	SchemaVersion int         `json:"schema_version"`
	State         string      `json:"state"`
	Results       any         `json:"results,omitempty"`
	Diagnostic    *diagnostic `json:"diagnostic,omitempty"`
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
			UIListen:      v.GetString("ui-listen"),
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
	emitWithVersion := func(version int, state string, result any) error {
		value := envelope{SchemaVersion: version, State: state, Results: result}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if cfg.Format == "text" {
			return printSummary(stdout, state, result)
		}
		return enc.Encode(value)
	}
	emit := func(state string, result any) error {
		return emitWithVersion(model.SchemaVersion, state, result)
	}
	withDB := func(ctx context.Context, fn func(*postgres.Store) error) error {
		s, err := postgres.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			if cfg.DatabaseURL == "" {
				return classified("missing_input", err)
			}
			return classified("storage_failure", err)
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
					return classified("storage_failure", err)
				}
				return emit("complete", map[string]string{"database": "schema current"})
			})
		},
	)
	runs := &cobra.Command{Use: "run", Short: "Inspect retained run state"}
	root.AddCommand(runs)
	command(runs, "recover", "Mark expired inspections interrupted and list runs that can resume", func(c *cobra.Command, _ []string) error {
		return withDB(c.Context(), func(s *postgres.Store) error {
			recoverable, err := s.ReconcileStale(c.Context())
			if err != nil {
				return classified("storage_failure", err)
			}
			return emit("complete", recoverable)
		})
	})
	resources := &cobra.Command{Use: "resources", Short: "Inspect configured resource budgets"}
	root.AddCommand(resources)
	command(resources, "status", "Show cache, staging and evidence budgets", func(c *cobra.Command, _ []string) error {
		cacheBudget, err := cfg.Cache.Budget(c.Context())
		if err != nil {
			return classified("storage_failure", err)
		}
		stagingBudget, err := inspection.DefaultStagingBudget()
		if err != nil {
			return classified("storage_failure", err)
		}
		return emit("complete", map[string]any{
			"cache":                            cacheBudget,
			"disk_staging":                     stagingBudget,
			"evidence_limit_bytes":             cfg.EvidenceLimit,
			"manifest_limit_bytes":             inspection.ManifestLimit,
			"device_tree_limit_bytes":          inspection.TreeLimit,
			"kernel_limit_bytes":               inspection.KernelLimit,
			"inspection_parallelism_per_cache": 1,
		})
	})
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
					return classified("storage_failure", err)
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
			if mode != "discover" && (req.Product == "" || (req.Version == "" && req.Build == "")) {
				return classified("missing_input", errors.New("probe and fetch require --product and --version or --build"))
			}
			return withDB(c.Context(), func(s *postgres.Store) (commandErr error) {
				var progressID string
				if mode == "fetch" {
					progressID, commandErr = s.StartAcquisitionProgress(
						c.Context(), req.Product, req.Version, req.Build)
					if commandErr != nil {
						return classified("storage_failure", commandErr)
					}
					fmt.Fprintf(stderr, "Acquisition attempt %s\n", progressID)
					defer func() {
						if commandErr == nil {
							return
						}
						state := "failed"
						if classify(commandErr) == "cancelled" {
							state = "cancelled"
						}
						cleanup, cancel := context.WithTimeout(context.WithoutCancel(c.Context()), 5*time.Second)
						defer cancel()
						if err := s.AdvanceAcquisitionProgress(cleanup, progressID,
							postgres.AcquisitionProgressUpdate{
								Phase: state, State: state, DiagnosticCode: classify(commandErr),
							}); err != nil {
							commandErr = classified("storage_failure",
								fmt.Errorf("%w; persist terminal progress: %v", commandErr, err))
						}
					}()
				}
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
						return classified("storage_failure", err)
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
					return classified("invalid_selection", err)
				}
				if mode == "fetch" {
					if err := s.AdvanceAcquisitionProgress(c.Context(), progressID,
						postgres.AcquisitionProgressUpdate{Phase: "probing"}); err != nil {
						return classified("storage_failure", err)
					}
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
					return classified("acquisition_failure", err)
				}
				if selected.Size > 0 && selected.Size != remote.Size() {
					return classified("unsupported_or_corrupt", fmt.Errorf(
						"%w: index %d, Apple CDN %d",
						errCandidateSizeMismatch,
						selected.Size,
						remote.Size(),
					))
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
					return classified("unsupported_or_corrupt", err)
				}
				info := remote.Info()
				selected.Size = info.Size
				selected.ETag = info.ETag
				if mode == "fetch" {
					fingerprint := model.Digest([]byte(fmt.Sprintf("%s\x00%s\x00%d",
						selected.URL, selected.ETag, selected.Size)))
					if err := s.AdvanceAcquisitionProgress(c.Context(), progressID,
						postgres.AcquisitionProgressUpdate{
							Phase: "selected", TotalBytes: &selected.Size,
							SourceFingerprint: fingerprint,
						}); err != nil {
						return classified("storage_failure", err)
					}
				}
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
					return classified("storage_failure", err)
				}
				if mode == "probe" {
					return emit("complete", map[string]any{
						"candidate": selected, "range": info, "manifest": probe,
						"full_archive_verified": false,
					})
				}
				fetchPlan, err := cfg.Cache.PlanFetch(c.Context(), selected)
				if err != nil {
					return classified("storage_failure", err)
				}
				fmt.Fprintf(stderr, "Fetch budget: %d additional bytes, %d available; cache %d/%d bytes, disk reserve %d bytes\n",
					fetchPlan.AdditionalBytes, fetchPlan.AvailableBytes,
					fetchPlan.UsedBytes, fetchPlan.LimitBytes, fetchPlan.ReserveBytes)
				if !fetchPlan.Fits {
					return classified("acquisition_failure", errors.New(fetchPlan.Reason))
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
					func(ctx context.Context, p acquisition.FetchProgress) error {
						return classified("storage_failure", s.AdvanceAcquisitionProgress(ctx,
							progressID, postgres.AcquisitionProgressUpdate{
								Phase: p.Phase, CompletedBytes: &p.DownloadedBytes,
								VerifiedBytes: &p.VerifiedBytes, TotalBytes: &p.TotalBytes,
							}))
					},
				)
				if err != nil {
					if classify(err) == "storage_failure" {
						return err
					}
					return classified("acquisition_failure", err)
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
					return classified("storage_failure", err)
				}
				if err := s.AdvanceAcquisitionProgress(c.Context(), progressID,
					postgres.AcquisitionProgressUpdate{Phase: "complete", State: "complete"}); err != nil {
					return classified("storage_failure", err)
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
					return classified("storage_failure", err)
				}
				defer release()
				a, err := inspection.Open(c.Context(), targetImage)
				if err != nil {
					return imageInput(err)
				}
				defer a.Close()
				if err = s.Register(c.Context(), a.Artifact, a.Targets); err != nil {
					return classified("storage_failure", err)
				}
				return emit("complete", a.Targets)
			})
		},
	)
	c.Flags().StringVar(&targetImage, "image", "", "IPSW path")
	_ = c.MarkFlagRequired("image")
	var inspectImage string
	var identity int
	var resumeID string
	c = command(
		root,
		"inspect",
		"Inspect a selected identity with durable component checkpoints",
		func(c *cobra.Command, _ []string) error {
			return withDB(c.Context(), func(s *postgres.Store) error {
				release, err := cfg.Cache.Lease(c.Context(), true)
				if err != nil {
					return classified("storage_failure", err)
				}
				defer release()
				fmt.Fprintln(stderr, "Hashing IPSW and reading BuildManifest…")
				a, err := inspection.Open(c.Context(), inspectImage)
				if err != nil {
					return imageInput(err)
				}
				defer a.Close()
				if resumeID != "" {
					prior, err := s.Run(c.Context(), resumeID)
					if err != nil {
						return classified("invalid_selection", err)
					}
					if prior.InputSHA256 != a.Artifact.SHA256 || prior.InputSize != a.Artifact.Size {
						return classified("changed_input", workflow.ErrResumeInputMismatch)
					}
				}
				if identity < 0 || identity >= len(a.Targets) {
					return classified("invalid_selection", errors.New("identity index is outside the BuildManifest"))
				}
				stagePlan, err := a.DiskStagingPlan(identity)
				if err != nil {
					return classified("storage_failure", err)
				}
				if stagePlan.RequiresStaging {
					fmt.Fprintf(stderr, "OS staging budget: %d declared bytes, %d available; reserve %d bytes\n",
						stagePlan.DeclaredBytes, stagePlan.AvailableBytes, stagePlan.ReserveBytes)
					if !stagePlan.Fits {
						fmt.Fprintln(stderr, "OS staging will report partial coverage:", stagePlan.Reason)
					}
				}
				var activeRun model.Run
				run, err := workflow.InspectBatched(
					c.Context(),
					s,
					a.Artifact,
					a.Targets,
					identity,
					resumeID,
					inspection.RunPlanVersion,
					inspection.Components,
					func(ctx context.Context, done map[string]bool, emit func(string, []model.Observation, model.Coverage) error) error {
						return a.InspectEach(ctx, identity, done, emit, func(component string) error {
							return classified("storage_failure",
								s.InspectionPhase(ctx, activeRun.ID, activeRun.LeaseToken, component))
						})
					},
					func(started model.Run) error { activeRun = started; return nil },
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
	c.Flags().StringVar(&resumeID, "resume", "", "Resume a run ID against this verified IPSW")
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
				f, err := s.FindingChains(c.Context(), targetID, runID)
				if err != nil {
					return classified("storage_failure", err)
				}
				return emitWithVersion(2, "complete", f)
			})
		},
	)
	c.Flags().StringVar(&runID, "run", "", "Run ID")
	c.Flags().StringVar(&targetID, "target", "", "Target ID")
	var evidenceID, findingID string
	c = command(
		finding,
		"show",
		"Show a finding revision chain or integrity-checked supporting bytes",
		func(c *cobra.Command, _ []string) error {
			if (evidenceID == "") == (findingID == "") {
				return classified("invalid_invocation", errFindingShowSelector)
			}
			return withDB(c.Context(), func(s *postgres.Store) error {
				if findingID != "" {
					chain, err := s.FindingChain(c.Context(), findingID)
					if err != nil {
						return err
					}
					return emitWithVersion(2, "complete", chain)
				}
				b, err := s.Evidence(c.Context(), evidenceID)
				if err != nil {
					return classified("storage_failure", err)
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
	c.Flags().StringVar(&findingID, "finding", "", "Stable finding ID")
	var draft model.RevisionDraft
	var value string
	var supportObservations, supportRevisions, contradictObservations, contradictRevisions []string
	c = command(
		finding,
		"revise",
		"Create a linked observed, inferred or unknown finding revision",
		func(c *cobra.Command, _ []string) error {
			if c.Flags().Changed("value") {
				draft.Value = json.RawMessage(value)
			}
			for _, id := range supportObservations {
				draft.References = append(
					draft.References,
					model.BasisReference{Role: "supporting", ObservationID: id},
				)
			}
			for _, id := range supportRevisions {
				draft.References = append(
					draft.References,
					model.BasisReference{Role: "supporting", RevisionID: id},
				)
			}
			for _, id := range contradictObservations {
				draft.References = append(
					draft.References,
					model.BasisReference{Role: "contradicting", ObservationID: id},
				)
			}
			for _, id := range contradictRevisions {
				draft.References = append(
					draft.References,
					model.BasisReference{Role: "contradicting", RevisionID: id},
				)
			}
			if err := draft.Validate(); err != nil {
				return classified("invalid_invocation", err)
			}
			return withDB(c.Context(), func(s *postgres.Store) error {
				receipt, err := s.AddRevision(c.Context(), draft)
				if err != nil {
					return err
				}
				return emitWithVersion(2, "complete", receipt)
			})
		},
	)
	c.Flags().StringVar(&draft.TargetID, "target", "", "Exact firmware target ID")
	c.Flags().StringVar(&draft.Subject, "subject", "", "Exact subject")
	c.Flags().StringVar(&draft.Property, "property", "", "Exact property")
	c.Flags().StringVar(&draft.Basis, "basis", "", "observed, inferred or unknown")
	c.Flags().
		StringVar(&draft.Presence, "presence", "", "For inference: present, absent or unexamined")
	c.Flags().StringVar(&value, "value", "", "JSON value for a present inference")
	c.Flags().StringVar(&draft.Rationale, "rationale", "", "Reason for this revision")
	c.Flags().StringVar(&draft.Author, "author", "cli", "Author or tool identity")
	c.Flags().
		StringVar(&draft.Supersedes, "supersedes", "", "Earlier revision ID in the same finding")
	c.Flags().
		StringVar(&draft.ObservationID, "observation", "", "Published direct observation for observed basis")
	c.Flags().
		StringSliceVar(&supportObservations, "support-observation", nil, "Supporting observation ID (repeatable)")
	c.Flags().
		StringSliceVar(&supportRevisions, "support-revision", nil, "Supporting revision ID (repeatable)")
	c.Flags().
		StringSliceVar(&contradictObservations, "contradict-observation", nil, "Contradicting observation ID (repeatable)")
	c.Flags().
		StringSliceVar(&contradictRevisions, "contradict-revision", nil, "Contradicting revision ID (repeatable)")
	var assessment model.AssessmentDraft
	c = command(finding, "assess", "Review one exact finding revision and its evidence basis", func(c *cobra.Command, _ []string) error {
		if err := assessment.Validate(); err != nil {
			return classified("invalid_invocation", err)
		}
		return withDB(c.Context(), func(s *postgres.Store) error {
			event, err := s.ReviewRevision(c.Context(), assessment)
			if errors.Is(err, postgres.ErrAssessmentConflict) {
				return classified("assessment_conflict", err)
			}
			if errors.Is(err, postgres.ErrAssessmentOwnership) {
				return classified("invalid_selection", err)
			}
			if err != nil {
				return classified("storage_failure", err)
			}
			return emitWithVersion(2, "complete", event)
		})
	})
	c.Flags().StringVar(&assessment.FindingID, "finding", "", "Stable finding ID")
	c.Flags().StringVar(&assessment.RevisionID, "revision", "", "Exact revision ID")
	c.Flags().StringVar(&assessment.Basis, "basis", "", "Revision basis: observed, inferred or unknown")
	c.Flags().StringVar(&assessment.State, "state", "", "Review state: accepted, rejected or disputed")
	c.Flags().StringVar(&assessment.Reviewer, "reviewer", "", "Reviewer identity")
	c.Flags().StringVar(&assessment.Reason, "reason", "", "Reviewer reason")
	c.Flags().Int64Var(&assessment.ExpectedID, "if-current", 0, "Current assessment event ID; 0 for an unreviewed revision")
	var left, right string
	c = command(
		root,
		"compare",
		"Compare exact subject/property observations from two retained runs",
		func(c *cobra.Command, _ []string) error {
			return withDB(c.Context(), func(s *postgres.Store) error {
				lr, err := s.Run(c.Context(), left)
				if err != nil {
					return classified("storage_failure", err)
				}
				rr, err := s.Run(c.Context(), right)
				if err != nil {
					return classified("storage_failure", err)
				}
				if (lr.State != "complete" && lr.State != "partial") ||
					(rr.State != "complete" && rr.State != "partial") {
					return errUnpublishedComparison
				}
				l, err := s.Findings(c.Context(), "", left)
				if err != nil {
					return classified("storage_failure", err)
				}
				r, err := s.Findings(c.Context(), "", right)
				if err != nil {
					return classified("storage_failure", err)
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
				return classified("storage_failure", err)
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
				return classified("storage_failure", err)
			}
			return emit("complete", map[string]any{"applied": apply, "entries": items})
		},
	)
	c.Flags().
		BoolVar(&apply, "apply", false, "Delete listed managed images after acquiring exclusive cache lease")
	addUICommand(root, v, &cfg, stderr)
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
