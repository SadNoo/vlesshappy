package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/SadNoo/vlesshappy/internal/config"
	"github.com/SadNoo/vlesshappy/internal/lifecycle"
	"github.com/SadNoo/vlesshappy/internal/realitykey"
	setupwizard "github.com/SadNoo/vlesshappy/internal/setup"
)

const version = "2.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "vlesshappy:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "run"
	if len(args) > 0 && (args[0] == "run" || args[0] == "validate" || args[0] == "keygen" || args[0] == "setup" || args[0] == "version") {
		command = args[0]
		args = args[1:]
	}
	if command == "version" {
		fmt.Println("vlesshappy", version)
		return nil
	}
	if command == "keygen" {
		return keygen(args)
	}
	if command == "setup" {
		return setup(args)
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	configPath := flags.String("config", "/etc/vlesshappy/config.json", "absolute path to config JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if command == "validate" {
		if err := lifecycle.Validate(ctx, cfg); err != nil {
			return err
		}
		fmt.Println("configuration is valid")
		return nil
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(cfg.LogLevel)}))
	return lifecycle.Run(ctx, cfg, logger)
}

func setup(args []string) error {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	dataDir := flags.String("data-dir", "/data", "absolute persistent data directory")
	volumeName := flags.String("volume-name", "vle-node-data", "Docker volume name shown in the run command")
	containerName := flags.String("container-name", "vle-node", "Docker container name shown in the run command")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return setupwizard.Run(ctx, setupwizard.Options{
		DataDir: *dataDir, VolumeName: *volumeName, ContainerName: *containerName,
		In: os.Stdin, Out: os.Stdout,
		ReadPassword: func() ([]byte, error) {
			return readHiddenPassword(os.Stdin, os.Stdout)
		},
	})
}

func keygen(args []string) error {
	flags := flag.NewFlagSet("keygen", flag.ContinueOnError)
	privatePath := flags.String("private-key-file", "", "absolute output path for the private key")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *privatePath == "" || !filepath.IsAbs(*privatePath) {
		return fmt.Errorf("keygen requires an absolute -private-key-file and no extra arguments")
	}
	privateText, publicText, err := realitykey.Generate(rand.Reader)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(*privatePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create private key file: %w", err)
	}
	if _, err := file.WriteString(privateText + "\n"); err != nil {
		file.Close()
		return fmt.Errorf("write private key file: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync private key file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close private key file: %w", err)
	}
	fmt.Println("REALITY public key:", publicText)
	return nil
}

func logLevel(value string) slog.Level {
	switch value {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
