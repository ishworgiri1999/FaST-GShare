package main

import (
	"context"
	"fmt"

	"github.com/KontonGu/FaST-GShare/proto/seti/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	addr := "localhost:5001"
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))

	if err != nil {
		panic(err)
	}
	defer conn.Close()
	client := seti.NewGPUConfiguratorServiceClient(conn)

	// Example usage of the client
	req := &seti.GetAvailableGPUsRequest{}
	resp, err := client.GetAvailableGPUs(context.Background(), req)
	if err != nil {
		panic(err)
	}

	fmt.Println(resp.Gpus)
}
