package main

import (
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	mobilev1 "github.com/fast-mobile-mcp/proto/gen/go/mobile/v1"
	"github.com/fast-mobile-mcp/worker-android/internal/config"
	"github.com/fast-mobile-mcp/worker-android/internal/server"
	"google.golang.org/grpc"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	lis, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		logger.Error("listen failed", "err", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	svc := server.NewMobileService(cfg, logger)
	mobilev1.RegisterMobileAutomationServiceServer(grpcServer, svc)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		svc.Close()
		grpcServer.GracefulStop()
	}()

	logger.Info("android worker started", "addr", cfg.ListenAddr)
	if err := grpcServer.Serve(lis); err != nil {
		logger.Error("grpc serve failed", "err", err)
		os.Exit(1)
	}
}
