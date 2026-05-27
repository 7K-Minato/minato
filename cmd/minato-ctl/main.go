package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

// ConsoleMessage represents a message in the WebSocket protocol
type ConsoleMessage struct {
	Type string `json:"type"`
	TS   int64  `json:"ts,omitempty"`
	Line string `json:"line,omitempty"`
	ID   string `json:"id,omitempty"`
	Data string `json:"data,omitempty"`
}

var (
	serverAddr string
	namespace  string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "minato-ctl",
		Short: "Minato control plane CLI",
	}

	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "http://localhost:8080", "Control plane API address")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "minato", "Default namespace")

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
				return getJSON(path.Join("/api/v1/gameservers"))
			},
		},
		&cobra.Command{
			Use:   "get [name]",
			Short: "Get a game server",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return getJSON(path.Join("/api/v1/gameservers", namespace, args[0]))
			},
		},
		&cobra.Command{
			Use:   "action [name] [action]",
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
				return postJSON(path.Join("/api/v1/gameservers", namespace, args[0], "actions", args[1]), params)
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
				return getJSON("/api/v1/gameserverfleets")
			},
		},
		&cobra.Command{
			Use:   "get [name]",
			Short: "Get a fleet",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return getJSON(path.Join("/api/v1/gameserverfleets", namespace, args[0]))
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
				return getJSON("/api/v1/profiles")
			},
		},
		&cobra.Command{
			Use:   "get [name]",
			Short: "Get a profile",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return getJSON(path.Join("/api/v1/profiles", args[0]))
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
				return getJSON(path.Join("/api/v1/gameservers", namespace, args[0], "snapshots"))
			},
		},
		&cobra.Command{
			Use:   "create [server]",
			Short: "Create a snapshot for a server",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return postJSON(path.Join("/api/v1/gameservers", namespace, args[0], "snapshots"), nil)
			},
		},
	)

	return cmd
}

func consoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "console [server]",
		Short: "Open interactive console to a game server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConsole(args[0])
		},
	}

	return cmd
}

func getJSON(path string) error {
	resp, err := http.Get(serverAddr + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, string(body))
	}

	pretty, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(pretty))
	return nil
}

func postJSON(path string, data interface{}) error {
	var body []byte
	if data != nil {
		var err error
		body, err = json.Marshal(data)
		if err != nil {
			return err
		}
	}

	resp, err := http.Post(serverAddr+path, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, string(respBody))
	}

	pretty, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(pretty))
	return nil
}

func runConsole(serverName string) error {
	u, err := url.Parse(serverAddr)
	if err != nil {
		return err
	}

	// Convert HTTP to WS
	wsScheme := "ws"
	if u.Scheme == "https" {
		wsScheme = "wss"
	}

	wsURL := fmt.Sprintf("%s://%s/api/v1/gameservers/%s/%s/console?namespace=%s",
		wsScheme, u.Host, namespace, serverName, namespace)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	fmt.Printf("Connected to %s console. Type commands, Ctrl+C to exit.\n", serverName)

	// Read server messages
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			var msg ConsoleMessage
			if err := conn.ReadJSON(&msg); err != nil {
				fmt.Fprintf(os.Stderr, "\nConnection closed: %v\n", err)
				return
			}

			switch msg.Type {
			case "log":
				tm := time.Unix(msg.TS, 0).Format("15:04:05")
				fmt.Printf("[%s] %s\n", tm, msg.Line)
			case "rcon-response":
				fmt.Printf("> %s\n", msg.Data)
			case "error":
				fmt.Fprintf(os.Stderr, "Error: %s\n", msg.Data)
			case "status":
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

		msg := ConsoleMessage{Type: "rcon", Data: input}
		if err := conn.WriteJSON(msg); err != nil {
			fmt.Fprintf(os.Stderr, "Send error: %v\n", err)
			break
		}
	}

	<-done
	return nil
}
