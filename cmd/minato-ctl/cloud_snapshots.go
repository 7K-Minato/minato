package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func cloudSnapshotsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshots",
		Short: "Manage snapshots of cloud servers",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list [server-id]",
			Short: "List snapshots of a server",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, tenantID, err := newCloudClientForTenant(cmd.Context())
				if err != nil {
					return err
				}
				snapshots, err := c.ListSnapshots(cmd.Context(), tenantID, args[0])
				if err != nil {
					return cloudErr(err)
				}
				if cloudJSON {
					return printJSON(snapshots)
				}
				rows := make([][]string, 0, len(snapshots))
				for _, s := range snapshots {
					var name, state, size, readyAt string
					if s.Metadata != nil {
						name = derefStr(s.Metadata.Name)
					}
					if s.Status != nil {
						state = derefStr((*string)(s.Status.State))
						size = derefStr(s.Status.Size)
						readyAt = derefTime(s.Status.ReadyAt)
					}
					rows = append(rows, []string{name, state, size, readyAt})
				}
				printTable([]string{"NAME", "STATE", "SIZE", "READY AT"}, rows)
				return nil
			},
		},
		&cobra.Command{
			Use:   "create [server-id]",
			Short: "Create a snapshot of a server",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, tenantID, err := newCloudClientForTenant(cmd.Context())
				if err != nil {
					return err
				}
				snapshot, err := c.CreateSnapshot(cmd.Context(), tenantID, args[0])
				if err != nil {
					return cloudErr(err)
				}
				if cloudJSON {
					return printJSON(snapshot)
				}
				name := ""
				if snapshot.Metadata != nil {
					name = derefStr(snapshot.Metadata.Name)
				}
				fmt.Printf("Snapshot %s created.\n", name)
				return nil
			},
		},
	)
	return cmd
}
