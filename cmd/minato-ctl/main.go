package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"

	"github.com/7k-minato/minato/sdk/controlplane"
)

var (
	serverAddr string
	namespace  string
	apiKey     string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "minato-ctl",
		Short: "Minato control plane CLI",
	}

	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "http://localhost:8080", "Control plane API address")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "minato", "Default namespace")
	rootCmd.PersistentFlags().StringVarP(&apiKey, "api-key", "k", os.Getenv("MINATO_API_KEY"), "API key (or MINATO_API_KEY)")

	rootCmd.AddCommand(
		serverCmd(),
		fleetCmd(),
		profileCmd(),
		snapshotCmd(),
		consoleCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newClient() (*controlplane.Client, error) {
	return controlplane.NewClient(serverAddr, apiKey, 30*time.Second)
}

func printJSON(v any) error {
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(pretty))
	return nil
}

func serverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Manage game servers",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List game servers",
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := newClient()
				if err != nil {
					return err
				}
				servers, err := c.ListGameServers(cmd.Context())
				if err != nil {
					return err
				}
				return printJSON(servers)
			},
		},
		&cobra.Command{
			Use:   "get [name]",
			Short: "Get a game server",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := newClient()
				if err != nil {
					return err
				}
				server, err := c.GetGameServer(cmd.Context(), namespace, args[0])
				if err != nil {
					return err
				}
				return printJSON(server)
			},
		},
		&cobra.Command{
			Use:   "action [name] [action] [key=value...]",
			Short: "Execute an action on a game server",
			Args:  cobra.MinimumNArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				params := map[string]string{}
				for i := 2; i < len(args); i++ {
					parts := strings.SplitN(args[i], "=", 2)
					if len(parts) == 2 {
						params[parts[0]] = parts[1]
					}
				}
				c, err := newClient()
				if err != nil {
					return err
				}
				ref, err := c.ExecuteAction(cmd.Context(), namespace, args[0], args[1], params)
				if err != nil {
					return err
				}
				return printJSON(ref)
			},
		},
	)

	return cmd
}

func fleetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "Manage game server fleets",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List fleets",
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := newClient()
				if err != nil {
					return err
				}
				fleets, err := c.ListGameServerFleets(cmd.Context())
				if err != nil {
					return err
				}
				return printJSON(fleets)
			},
		},
		&cobra.Command{
			Use:   "get [name]",
			Short: "Get a fleet",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := newClient()
				if err != nil {
					return err
				}
				fleet, err := c.GetGameServerFleet(cmd.Context(), namespace, args[0])
				if err != nil {
					return err
				}
				return printJSON(fleet)
			},
		},
	)

	return cmd
}

func profileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage game profiles",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List profiles",
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := newClient()
				if err != nil {
					return err
				}
				profiles, err := c.ListProfiles(cmd.Context())
				if err != nil {
					return err
				}
				return printJSON(profiles)
			},
		},
		&cobra.Command{
			Use:   "get [name]",
			Short: "Get a profile",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := newClient()
				if err != nil {
					return err
				}
				profile, err := c.GetProfile(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				return printJSON(profile)
			},
		},
	)

	return cmd
}

func snapshotCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Manage snapshots",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list [server]",
			Short: "List snapshots for a server",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := newClient()
				if err != nil {
					return err
				}
				snapshots, err := c.ListSnapshots(cmd.Context(), namespace, args[0])
				if err != nil {
					return err
				}
				return printJSON(snapshots)
			},
		},
		&cobra.Command{
			Use:   "create [server]",
			Short: "Create a snapshot for a server",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := newClient()
				if err != nil {
					return err
				}
				snapshot, err := c.CreateSnapshot(cmd.Context(), namespace, args[0])
				if err != nil {
					return err
				}
				return printJSON(snapshot)
			},
		},
	)

	return cmd
}

func consoleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "console [server]",
		Short: "Open interactive console to a game server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConsole(cmd.Context(), args[0])
		},
	}
}

func runConsole(ctx context.Context, serverName string) error {
	u, err := url.Parse(serverAddr)
	if err != nil {
		return err
	}

	// Convert HTTP to WS
	wsScheme := "ws"
	if u.Scheme == "https" {
		wsScheme = "wss"
	}

	wsURL := fmt.Sprintf("%s://%s/api/v1/gameservers/%s/%s/console", wsScheme, u.Host, namespace, serverName)

	header := http.Header{}
	if apiKey != "" {
		header.Set("Authorization", "Bearer "+apiKey)
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer func() { _ = conn.Close() }()

	fmt.Printf("Connected to %s console. Type commands, Ctrl+C to exit.\n", serverName)

	// Read server messages
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			var msg controlplane.ConsoleMessage
			if err := conn.ReadJSON(&msg); err != nil {
				fmt.Fprintf(os.Stderr, "\nConnection closed: %v\n", err)
				return
			}

			switch msg.Type {
			case controlplane.ConsoleTypeLog:
				tm := time.Unix(msg.TS, 0).Format("15:04:05")
				fmt.Printf("[%s] %s\n", tm, msg.Line)
			case controlplane.ConsoleTypeRconResponse:
				fmt.Printf("> %s\n", msg.Data)
			case controlplane.ConsoleTypeError:
				fmt.Fprintf(os.Stderr, "Error: %s\n", msg.Data)
			case controlplane.ConsoleTypeStatus:
				fmt.Printf("[Status: %s]\n", msg.Data)
			}
		}
	}()

	// Read user input
	for {
		var input string
		fmt.Print("> ")
		if _, err := fmt.Scanln(&input); err != nil {
			break
		}

		msg := controlplane.ConsoleMessage{Type: controlplane.ConsoleTypeRcon, Data: input}
		if err := conn.WriteJSON(msg); err != nil {
			fmt.Fprintf(os.Stderr, "Send error: %v\n", err)
			break
		}
	}

	<-done
	return nil
}
