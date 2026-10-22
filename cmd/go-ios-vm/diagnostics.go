package main

import (
	"archive/zip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/deploymenttheory/go-ios-vm/internal/acquisition"
	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

type diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type classifiedError struct {
	code string
	err  error
}

func (e *classifiedError) Error() string { return e.err.Error() }
func (e *classifiedError) Unwrap() error { return e.err }

func classified(code string, err error) error {
	if err == nil {
		return nil
	}
	return &classifiedError{code: code, err: err}
}

func imageInput(err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, os.ErrNotExist) {
		return classified("missing_input", err)
	}
	if errors.Is(err, os.ErrPermission) {
		return classified("storage_failure", err)
	}
	return classified("unsupported_or_corrupt", err)
}

func classify(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "cancelled"
	}
	var typed *classifiedError
	if errors.As(err, &typed) {
		return typed.code
	}
	if errors.Is(err, os.ErrNotExist) {
		return "missing_input"
	}
	if errors.Is(err, zip.ErrFormat) {
		return "unsupported_or_corrupt"
	}
	message := err.Error()
	if strings.Contains(message, "required flag") || strings.Contains(message, "requires --") {
		return "missing_input"
	}
	if errors.Is(err, errRunOrTargetIsRequired) {
		return "missing_input"
	}
	if errors.Is(err, errUnpublishedComparison) {
		return "invalid_selection"
	}
	if errors.Is(err, errFormatMustBeTextOrJson) || errors.Is(err, errInvalidCacheBudget) ||
		strings.HasPrefix(message, "unknown command") || strings.HasPrefix(message, "unknown flag") ||
		strings.HasPrefix(message, "accepts ") || strings.HasPrefix(message, "invalid argument") {
		return "invalid_invocation"
	}
	return "operation_failed"
}

var (
	credentialURI = regexp.MustCompile(`(?i)\bpostgres(?:ql)?://[^\s]+`)
	credentialKV  = regexp.MustCompile(`(?i)\b(password|passwd|pwd)\s*=\s*(?:'[^']*'|"[^"]*"|[^\s]+)`)
)

func safeDiagnostic(err error) diagnostic {
	message := credentialURI.ReplaceAllString(err.Error(), "postgres://[REDACTED]")
	message = credentialKV.ReplaceAllString(message, "${1}=[REDACTED]")
	if classify(err) == "storage_failure" && strings.Contains(strings.ToLower(message), "postgres") {
		// Driver messages can contain connection parameters outside a URL.
		message = "database operation failed"
	}
	return diagnostic{Code: classify(err), Message: message}
}

func printSummary(w io.Writer, state string, result any) error {
	var b strings.Builder
	label := strings.ToUpper(state)
	switch v := result.(type) {
	case map[string]string:
		fmt.Fprintf(&b, "%s: database %s\n", label, v["database"])
	case acquisition.Listing:
		printFirmwareListing(&b, state, v)
	case []acquisition.Response:
		candidates, failed := 0, 0
		for _, source := range v {
			candidates += len(source.Candidates)
			if source.Status == "failed" {
				failed++
			}
		}
		fmt.Fprintf(&b, "%s: %d catalog sources, %d candidates, %d failed\n", label, len(v), candidates, failed)
		for _, source := range v {
			fmt.Fprintf(&b, "  %s: %s", source.Status, source.Source)
			if source.Error != "" {
				fmt.Fprintf(&b, " (%s)", source.Error)
			}
			b.WriteByte('\n')
		}
	case model.Artifact:
		fmt.Fprintf(&b, "%s: verified IPSW %s\n  %d bytes, SHA-256 %s\n", label, v.Path, v.Size, v.SHA256)
	case []model.Target:
		fmt.Fprintf(&b, "%s: %d BuildManifest identities\n", label, len(v))
		for _, target := range v {
			fmt.Fprintf(&b, "  %d: %s %s (%s), board %s\n", target.Identity, strings.Join(target.Products, ", "), target.Version, target.Build, target.Board)
		}
	case model.Run:
		fmt.Fprintf(&b, "%s: inspection run %s, target %s\n", label, v.ID, v.TargetID)
		for _, coverage := range v.Coverage {
			fmt.Fprintf(&b, "  %s: %s", coverage.Component, coverage.State)
			if coverage.Reason != "" {
				fmt.Fprintf(&b, " (%s)", coverage.Reason)
			}
			b.WriteByte('\n')
		}
	case []model.Finding:
		fmt.Fprintf(&b, "%s: %d findings\n", label, len(v))
		for i, finding := range v {
			if i == 20 {
				fmt.Fprintf(&b, "  … %d more; use --format json for all findings\n", len(v)-i)
				break
			}
			fmt.Fprintf(&b, "  %s %s: %s\n", finding.Observation.Subject, finding.Observation.Property, finding.ID)
		}
	case []acquisition.Entry:
		fmt.Fprintf(&b, "%s: %d managed cache entries\n", label, len(v))
		for _, entry := range v {
			fmt.Fprintf(&b, "  %s (%d bytes)\n", entry.Path, entry.Size)
		}
	case map[string]any:
		switch {
		case v["manifest"] != nil:
			candidate, _ := v["candidate"].(acquisition.Candidate)
			fmt.Fprintf(&b, "%s: BuildManifest matches %s %s (%s); full archive unverified\n", label, candidate.Product, candidate.Version, candidate.Build)
		case v["data"] != nil:
			fmt.Fprintf(&b, "%s: evidence %v (%v bytes, base64)\n", label, v["evidence_id"], v["size"])
		case v["changes"] != nil:
			changes, _ := v["changes"].([]model.Change)
			fmt.Fprintf(&b, "%s: %d exact subject/property changes\n", label, len(changes))
			for i, change := range changes {
				if i == 20 {
					fmt.Fprintf(&b, "  … %d more; use --format json for all changes\n", len(changes)-i)
					break
				}
				fmt.Fprintf(&b, "  %s %s: %s\n", change.Subject, change.Property, change.State)
			}
		case v["entries"] != nil:
			entries, _ := v["entries"].([]acquisition.Entry)
			action := "would prune"
			if v["applied"] == true {
				action = "pruned"
			}
			fmt.Fprintf(&b, "%s: %s %d managed cache entries\n", label, action, len(entries))
			for _, entry := range entries {
				fmt.Fprintf(&b, "  %s (%d bytes)\n", entry.Path, entry.Size)
			}
		default:
			fmt.Fprintf(&b, "%s\n", label)
		}
	default:
		fmt.Fprintf(&b, "%s\n", label)
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return err
	}
	if value, ok := result.(map[string]any); ok {
		if data, ok := value["data"].([]byte); ok {
			encoder := base64.NewEncoder(base64.StdEncoding, w)
			if _, err := encoder.Write(data); err != nil {
				return err
			}
			if err := encoder.Close(); err != nil {
				return err
			}
			_, err := io.WriteString(w, "\n")
			return err
		}
	}
	return nil
}

func writeDiagnostic(w io.Writer, format string, err error) error {
	d := safeDiagnostic(err)
	if format == "json" {
		state := "failed"
		if d.Code == "cancelled" {
			state = "cancelled"
		}
		return json.NewEncoder(w).Encode(envelope{
			SchemaVersion: model.SchemaVersion, State: state, Diagnostic: &d,
		})
	}
	_, writeErr := fmt.Fprintf(w, "Error [%s]: %s\n", d.Code, d.Message)
	return writeErr
}
