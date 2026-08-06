package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func cloudCatalogCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "catalog",
		Short: "Show the game catalog (profiles, tiers, regions)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, tenantID, err := newCloudClientForTenant(cmd.Context())
			if err != nil {
				return err
			}
			entries, err := c.GetCatalog(cmd.Context(), tenantID)
			if err != nil {
				return cloudErr(err)
			}
			if cloudJSON {
				return printJSON(entries)
			}
			rows := make([][]string, 0, len(entries))
			for _, e := range entries {
				tiers := make([]string, 0, len(e.Tiers))
				for _, t := range e.Tiers {
					tiers = append(tiers, t.Name)
				}
				rows = append(rows, []string{
					e.Name,
					e.DisplayName,
					derefStr(e.Category),
					strings.Join(tiers, ","),
					strings.Join(e.Regions, ","),
				})
			}
			printTable([]string{"NAME", "DISPLAY NAME", "CATEGORY", "TIERS", "REGIONS"}, rows)
			return nil
		},
	}
}

func cloudPlansCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "plans",
		Short: "List subscription plans",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newCloudClient()
			if err != nil {
				return err
			}
			plans, err := c.ListPlans(cmd.Context())
			if err != nil {
				return cloudErr(err)
			}
			if cloudJSON {
				return printJSON(plans)
			}
			rows := make([][]string, 0, len(plans))
			for _, p := range plans {
				price := ""
				if p.MonthlyPriceCents != nil {
					price = fmt.Sprintf("%.2f", float64(*p.MonthlyPriceCents)/100)
				}
				rows = append(rows, []string{
					p.Id,
					p.DisplayName,
					fmt.Sprint(p.MaxServers),
					fmt.Sprint(p.MaxStorageGb),
					string(p.Isolation),
					price,
				})
			}
			printTable([]string{"ID", "NAME", "MAX SERVERS", "MAX STORAGE (GB)", "ISOLATION", "PRICE/MONTH"}, rows)
			return nil
		},
	}
}

func cloudSubscriptionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "subscription",
		Short: "Show the tenant's subscription",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, tenantID, err := newCloudClientForTenant(cmd.Context())
			if err != nil {
				return err
			}
			sub, err := c.GetSubscription(cmd.Context(), tenantID)
			if err != nil {
				return cloudErr(err)
			}
			if cloudJSON {
				return printJSON(sub)
			}
			printTable(
				[]string{"TENANT", "PLAN", "STATUS", "PERIOD END", "GRACE UNTIL"},
				[][]string{{
					sub.TenantId,
					sub.PlanId,
					sub.Status,
					derefTime(sub.CurrentPeriodEnd),
					derefTime(sub.GraceUntil),
				}},
			)
			return nil
		},
	}
}
