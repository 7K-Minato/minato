package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/7k-minato/minato/internal/cloudapi"
)

var (
	cloudURL    string
	cloudTenant string
	cloudJSON   bool
)

// cloudStdin is where interactive prompts (login) read from; tests override it.
var cloudStdin io.Reader = os.Stdin

func cloudCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cloud",
		Short: "Manage servers on a minato-cloud (SaaS) deployment",
	}
	cmd.PersistentFlags().StringVar(&cloudURL, "url", "", "Cloud API URL (or MINATO_CLOUD_URL, default http://localhost:8080)")
	cmd.PersistentFlags().StringVar(&cloudTenant, "tenant", "", "Tenant id, slug or name (defaults to your only tenant)")
	cmd.PersistentFlags().BoolVar(&cloudJSON, "json", false, "Output raw JSON instead of a table")

	cmd.AddCommand(
		cloudLoginCmd(),
		cloudLogoutCmd(),
		cloudWhoamiCmd(),
		cloudServersCmd(),
		cloudSnapshotsCmd(),
		cloudActionsCmd(),
		cloudCatalogCmd(),
		cloudPlansCmd(),
		cloudSubscriptionCmd(),
		cloudAPIKeysCmd(),
	)
	return cmd
}

func newCloudClient() (*cloudapi.Client, error) {
	cfg, err := loadCloudConfig()
	if err != nil {
		return nil, err
	}
	token, _ := resolveCloudToken(cfg)
	return cloudapi.NewClient(resolveCloudURL(cfg), token, 30*time.Second)
}

// newCloudClientForTenant builds a client and resolves --tenant to a tenant id.
func newCloudClientForTenant(ctx context.Context) (*cloudapi.Client, string, error) {
	c, err := newCloudClient()
	if err != nil {
		return nil, "", err
	}
	t, err := c.ResolveTenant(ctx, cloudTenant)
	if err != nil {
		return nil, "", cloudErr(err)
	}
	return c, t.Id, nil
}

// cloudErr maps cloud API errors to actionable CLI messages.
func cloudErr(err error) error {
	var ae *cloudapi.APIError
	if !errors.As(err, &ae) {
		return err
	}
	switch ae.Status {
	case 401:
		return fmt.Errorf("not authenticated (%s) — run `minato-ctl cloud login`", ae.Message)
	case 402:
		return fmt.Errorf("payment required (%s) — subscribe to a plan first (see `minato-ctl cloud plans`)", ae.Message)
	case 403:
		return fmt.Errorf("forbidden (%s) — check your tenant role, quota or API key scopes", ae.Message)
	case 404:
		return fmt.Errorf("not found: %s", ae.Message)
	default:
		return err
	}
}

func printTable(header []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(header, "\t"))
	for _, r := range rows {
		fmt.Fprintln(w, strings.Join(r, "\t"))
	}
	_ = w.Flush()
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// acquireSessionToken obtains a Keycloak session token interactively. Today
// it asks the user to paste an ID token from the web dashboard; it is a
// variable so an OIDC device flow can replace it later without touching the
// login command.
var acquireSessionToken = func(ctx context.Context) (string, error) {
	fmt.Print("Paste a Keycloak ID token (from the cloud dashboard): ")
	tok, err := bufio.NewReader(cloudStdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return "", fmt.Errorf("no token provided")
	}
	return tok, nil
}

func cloudLoginCmd() *cobra.Command {
	var apiKeyFlag string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store credentials for a minato-cloud deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadCloudConfig()
			if err != nil {
				return err
			}
			url := resolveCloudURL(cfg)
			cfg.Cloud.URL = url

			if apiKeyFlag != "" {
				if !strings.HasPrefix(apiKeyFlag, "mk_") {
					return fmt.Errorf("tenant API keys start with \"mk_\" — got %q", apiKeyFlag)
				}
				cfg.Cloud.APIKey = apiKeyFlag
				cfg.Cloud.SessionToken = ""
			} else {
				tok, err := acquireSessionToken(cmd.Context())
				if err != nil {
					return err
				}
				cfg.Cloud.SessionToken = tok
				cfg.Cloud.APIKey = ""
			}

			// Verify the credential before persisting it.
			token, mode := resolveCloudToken(cfg)
			c, err := cloudapi.NewClient(url, token, 30*time.Second)
			if err != nil {
				return err
			}
			tenants, err := c.ListMyTenants(cmd.Context())
			if err != nil {
				return cloudErr(err)
			}
			if err := saveCloudConfig(cfg); err != nil {
				return err
			}

			fmt.Printf("Logged in to %s (%s). Credentials stored in %s\n", url, mode, cloudConfigPath)
			rows := make([][]string, 0, len(tenants))
			for _, t := range tenants {
				rows = append(rows, []string{t.Id, t.Slug, t.Name})
			}
			printTable([]string{"TENANT ID", "SLUG", "NAME"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&apiKeyFlag, "api-key", "", "Tenant API key (mk_...); prompts for a session token when omitted")
	return cmd
}

func cloudLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove stored cloud credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadCloudConfig()
			if err != nil {
				return err
			}
			cfg.Cloud.APIKey = ""
			cfg.Cloud.SessionToken = ""
			if err := saveCloudConfig(cfg); err != nil {
				return err
			}
			fmt.Println("Logged out; stored credentials removed.")
			return nil
		},
	}
}

func cloudWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show cloud URL, auth mode and tenants",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadCloudConfig()
			if err != nil {
				return err
			}
			url := resolveCloudURL(cfg)
			token, mode := resolveCloudToken(cfg)
			c, err := cloudapi.NewClient(url, token, 30*time.Second)
			if err != nil {
				return err
			}
			tenants, err := c.ListMyTenants(cmd.Context())
			if err != nil {
				return cloudErr(err)
			}
			if cloudJSON {
				return printJSON(map[string]any{
					"url":      url,
					"authMode": mode,
					"tenants":  tenants,
				})
			}
			fmt.Printf("URL:  %s\nAuth: %s\n", url, mode)
			rows := make([][]string, 0, len(tenants))
			for _, t := range tenants {
				rows = append(rows, []string{t.Id, t.Slug, t.Name})
			}
			printTable([]string{"TENANT ID", "SLUG", "NAME"}, rows)
			return nil
		},
	}
}
