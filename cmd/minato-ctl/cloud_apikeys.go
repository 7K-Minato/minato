package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/7k-minato/minato/internal/cloudapi"
)

func cloudAPIKeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apikeys",
		Short: "Manage tenant API keys (session auth only)",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List API keys",
			RunE: func(cmd *cobra.Command, args []string) error {
				c, tenantID, err := newCloudClientForTenant(cmd.Context())
				if err != nil {
					return err
				}
				keys, err := c.ListAPIKeys(cmd.Context(), tenantID)
				if err != nil {
					return cloudErr(err)
				}
				if cloudJSON {
					return printJSON(keys)
				}
				rows := make([][]string, 0, len(keys))
				for _, k := range keys {
					scopes := make([]string, 0, len(k.Scopes))
					for _, s := range k.Scopes {
						scopes = append(scopes, string(s))
					}
					rows = append(rows, []string{
						k.Id,
						k.Name,
						k.Prefix,
						strings.Join(scopes, ","),
						derefTime(k.ExpiresAt),
						k.CreatedAt.Format("2006-01-02"),
					})
				}
				printTable([]string{"ID", "NAME", "PREFIX", "SCOPES", "EXPIRES", "CREATED"}, rows)
				return nil
			},
		},
		cloudAPIKeyCreateCmd(),
		&cobra.Command{
			Use:   "delete [id]",
			Short: "Delete an API key",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, tenantID, err := newCloudClientForTenant(cmd.Context())
				if err != nil {
					return err
				}
				if err := c.DeleteAPIKey(cmd.Context(), tenantID, args[0]); err != nil {
					return cloudErr(err)
				}
				fmt.Printf("API key %s deleted.\n", args[0])
				return nil
			},
		},
	)
	return cmd
}

func cloudAPIKeyCreateCmd() *cobra.Command {
	var name string
	var scopes []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an API key (prints the key exactly once)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, tenantID, err := newCloudClientForTenant(cmd.Context())
			if err != nil {
				return err
			}
			req := cloudapi.CreateAPIKeyRequest{Name: name}
			if len(scopes) > 0 {
				ss := make([]cloudapi.CreateAPIKeyRequestScopes, 0, len(scopes))
				for _, s := range scopes {
					ss = append(ss, cloudapi.CreateAPIKeyRequestScopes(s))
				}
				req.Scopes = &ss
			}
			key, err := c.CreateAPIKey(cmd.Context(), tenantID, req)
			if err != nil {
				return cloudErr(err)
			}
			if cloudJSON {
				return printJSON(key)
			}
			fmt.Printf("API key %q created. Store this key now — it is shown only once:\n%s\n", key.Name, key.Key)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Key name (required)")
	cmd.Flags().StringSliceVar(&scopes, "scope", nil, "Scopes (servers:read,servers:write,console,snapshots,*); empty grants all")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}
