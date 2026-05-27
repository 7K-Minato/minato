package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	agentv1 "github.com/7k-group/minato/api/agent/v1/minato/agent/v1"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9876", "agent gRPC address")
	flag.Parse()

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("connect error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	client := agentv1.NewAgentClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Info(ctx, &agentv1.InfoRequest{}); err != nil {
		fmt.Printf("Info failed: %v\n", err)
		os.Exit(1)
	}
	if _, err := client.HealthCheck(ctx, &agentv1.HealthRequest{}); err != nil {
		fmt.Printf("HealthCheck failed: %v\n", err)
		os.Exit(1)
	}
	if _, err := client.GetPlayers(ctx, &agentv1.PlayersRequest{}); err != nil {
		fmt.Printf("GetPlayers failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("conformance ok")
}
