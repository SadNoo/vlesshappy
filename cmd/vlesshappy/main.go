package main

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/SadNoo/vlesshappy/internal/config"
	"github.com/SadNoo/vlesshappy/internal/lifecycle"
)

const version = "2.0.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "vlesshappy:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "run"
	if len(args) > 0 && (args[0] == "run" || args[0] == "validate" || args[0] == "keygen" || args[0] == "version") {
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

func keygen(args []string) error {
	flags := flag.NewFlagSet("keygen", flag.ContinueOnError)
	privatePath := flags.String("private-key-file", "", "absolute output path for the private key")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *privatePath == "" || !filepath.IsAbs(*privatePath) {
		return fmt.Errorf("keygen requires an absolute -private-key-file and no extra arguments")
	}
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate X25519 key: %w", err)
	}
	privateText := base64.RawURLEncoding.EncodeToString(key.Bytes()) + "\n"
	file, err := os.OpenFile(*privatePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create private key file: %w", err)
	}
	if _, err := file.WriteString(privateText); err != nil {
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
	publicText := base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
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
