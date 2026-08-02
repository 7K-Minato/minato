package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func cloudActionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "actions",
		Short: "List and run actions on cloud servers",
	}

	var params map[string]string
	run := &cobra.Command{
		Use:   "run [server-id] [action]",
		Short: "Execute an action on a server",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, tenantID, err := newCloudClientForTenant(cmd.Context())
			if err != nil {
				return err
			}
			ref, err := c.ExecuteAction(cmd.Context(), tenantID, args[0], args[1], params)
			if err != nil {
				return cloudErr(err)
			}
			if cloudJSON {
				return printJSON(ref)
			}
			fmt.Printf("Action %q started on server %s (execution %s).\n", args[1], args[0], ref.Name)
			return nil
		},
	}
	run.Flags().StringToStringVar(&params, "param", nil, "Action parameters, e.g. --param message=hi --param reason=x")

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list [server-id]",
			Short: "List actions available on a server",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, tenantID, err := newCloudClientForTenant(cmd.Context())
				if err != nil {
					return err
				}
				actions, err := c.ListActions(cmd.Context(), tenantID, args[0])
				if err != nil {
					return cloudErr(err)
				}
				if cloudJSON {
					return printJSON(actions)
				}
				rows := make([][]string, 0, len(actions))
				for _, a := range actions {
					paramNames := []string{}
					if a.Params != nil {
						for p := range *a.Params {
							paramNames = append(paramNames, p)
						}
						sort.Strings(paramNames)
					}
					rows = append(rows, []string{derefStr(a.Name), derefStr(a.Description), strings.Join(paramNames, ",")})
				}
				printTable([]string{"NAME", "DESCRIPTION", "PARAMS"}, rows)
				return nil
			},
		},
		run,
	)
	return cmd
}
