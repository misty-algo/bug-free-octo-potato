// arrival-schema validates MongoDB topology and indexes. Index creation is explicit.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	orderrepo "github.com/BwCloudWeGo/bw-cli/internal/order/repo"
	"github.com/BwCloudWeGo/bw-cli/pkg/config"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
)

type schemaOptions struct {
	apply      bool
	configPath string
}

func parseOptions(args []string) (schemaOptions, error) {
	var o schemaOptions
	fs := flag.NewFlagSet("arrival-schema", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&o.apply, "apply", false, "explicitly create required collections and indexes")
	fs.StringVar(&o.configPath, "config", "configs/config.yaml", "local configuration path")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() != 0 {
		return o, errors.New("unexpected positional argument")
	}
	return o, nil
}
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	o, err := parseOptions(args)
	if err != nil {
		return err
	}
	cfg, err := config.Load(o.configPath)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	if v := os.Getenv("ARRIVAL_MONGO_URI"); v != "" {
		cfg.MongoDB.URI = v
		cfg.MongoDB.Username = ""
		cfg.MongoDB.Password = ""
	}
	if v := os.Getenv("ARRIVAL_MONGO_DATABASE"); v != "" {
		cfg.MongoDB.Database = v
	}
	if cfg.MongoDB.Database == "" {
		return errors.New("MongoDB database is required")
	}
	c, err := mongox.NewClient(cfg.MongoDB.MongoxConfig())
	if err != nil {
		return errors.New("MongoDB client configuration invalid; check local configuration")
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := c.Disconnect(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "MongoDB disconnect failed")
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := c.Database(cfg.MongoDB.Database)
	if o.apply {
		if err := orderrepo.MigrateMongoArrivalSchema(ctx, db); err != nil {
			return fmt.Errorf("arrival schema migration: %w", err)
		}
	}
	if err := orderrepo.ValidateMongoArrivalSchema(ctx, db); err != nil {
		return fmt.Errorf("arrival schema validation: %w", err)
	}
	fmt.Fprintln(os.Stdout, "Arrival MongoDB topology and required indexes validated.")
	return nil
}
