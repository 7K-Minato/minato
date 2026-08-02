package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/7k-minato/minato/internal/cloudapi"
)

func cloudServersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "servers",
		Short: "Manage cloud game servers",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List servers",
			RunE: func(cmd *cobra.Command, args []string) error {
				c, tenantID, err := newCloudClientForTenant(cmd.Context())
				if err != nil {
					return err
				}
				servers, err := c.ListServers(cmd.Context(), tenantID)
				if err != nil {
					return cloudErr(err)
				}
				if cloudJSON {
					return printJSON(servers)
				}
				rows := make([][]string, 0, len(servers))
				for _, s := range servers {
					rows = append(rows, []string{s.Id, s.Name, s.Profile, derefStr(s.Tier), s.Status, strconv.FormatBool(derefBool(s.Suspended))})
				}
				printTable([]string{"ID", "NAME", "PROFILE", "TIER", "STATUS", "SUSPENDED"}, rows)
				return nil
			},
		},
		&cobra.Command{
			Use:   "get [id]",
			Short: "Get a server",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, tenantID, err := newCloudClientForTenant(cmd.Context())
				if err != nil {
					return err
				}
				server, err := c.GetServer(cmd.Context(), tenantID, args[0])
				if err != nil {
					return cloudErr(err)
				}
				return printJSON(server)
			},
		},
		cloudServerCreateCmd(),
		&cobra.Command{
			Use:   "delete [id]",
			Short: "Delete a server",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, tenantID, err := newCloudClientForTenant(cmd.Context())
				if err != nil {
					return err
				}
				if err := c.DeleteServer(cmd.Context(), tenantID, args[0]); err != nil {
					return cloudErr(err)
				}
				fmt.Printf("Server %s deleted.\n", args[0])
				return nil
			},
		},
	)
	return cmd
}

func cloudServerCreateCmd() *cobra.Command {
	var name, profile, tier, region, storage string
	var env map[string]string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a server",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, tenantID, err := newCloudClientForTenant(cmd.Context())
			if err != nil {
				return err
			}
			req := cloudapi.CreateServerRequest{
				Name:    name,
				Profile: profile,
			}
			if tier != "" {
				req.Tier = &tier
			}
			if region != "" {
				req.Region = &region
			}
			if storage != "" {
				req.StorageSize = &storage
			}
			if len(env) > 0 {
				req.Env = &env
			}
			server, err := c.CreateServer(cmd.Context(), tenantID, req)
			if err != nil {
				return cloudErr(err)
			}
			if cloudJSON {
				return printJSON(server)
			}
			fmt.Printf("Server %s (%s) created: profile=%s status=%s\n", server.Name, server.Id, server.Profile, server.Status)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Server name (required)")
	cmd.Flags().StringVar(&profile, "profile", "", "Game profile name (required)")
	cmd.Flags().StringVar(&tier, "tier", "", "Resource tier (profile default when omitted)")
	cmd.Flags().StringVar(&region, "region", "", "Region to deploy in")
	cmd.Flags().StringVar(&storage, "storage", "", "PVC size, e.g. 20Gi")
	cmd.Flags().StringToStringVar(&env, "env", nil, "Environment overrides, e.g. --env KEY=value,OTHER=x")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("profile")
	return cmd
}

func derefBool(b *bool) bool {
	return b != nil && *b
}
