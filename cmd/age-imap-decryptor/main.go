package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/age-imap-decryptor/age-imap-decryptor/internal/config"
	"github.com/age-imap-decryptor/age-imap-decryptor/internal/crypto"
	"github.com/age-imap-decryptor/age-imap-decryptor/internal/mail"
	"github.com/age-imap-decryptor/age-imap-decryptor/internal/processor"
)

// buildVersion is set at link time via -X main.buildVersion=<tag>.
var buildVersion = "dev"

const (
	idleMaxRetries  = 5
	idleBackoffBase = time.Second
	idleBackoffMax  = 60 * time.Second
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	scanFlag := flag.Bool("scan", false, "override runtime.initial_scan from config")
	idleFlag := flag.Bool("idle", false, "override runtime.idle from config")
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		os.Stdout.WriteString(buildVersion + "\n")
		return
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Error("load config", "err", err)
		os.Exit(1)
	}

	// Only apply flag overrides when the flag was explicitly provided on the
	// command line; otherwise the config value remains authoritative.
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "scan":
			cfg.Runtime.InitialScan = *scanFlag
		case "idle":
			cfg.Runtime.Idle = *idleFlag
		}
	})

	if !cfg.Runtime.InitialScan && !cfg.Runtime.Idle {
		log.Error("invalid mode: neither initial_scan nor idle is active")
		os.Exit(1)
	}

	dec, err := crypto.LoadIdentities(cfg.Keys.Path)
	if err != nil {
		log.Error("load age identities", "err", err)
		os.Exit(1)
	}

	client, err := mail.Connect(cfg.IMAP.Server, cfg.IMAP.Port, cfg.IMAP.Username, cfg.IMAP.Password)
	if err != nil {
		log.Error("connect to IMAP server", "err", err)
		os.Exit(1)
	}
	defer client.Close()

	if err := client.SelectFolder(cfg.IMAP.Mailbox); err != nil {
		log.Error("select mailbox", "err", err)
		os.Exit(1)
	}

	proc := processor.New(dec, cfg.Output.BaseDir, cfg.Filter.AttachmentExtension)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.Runtime.InitialScan {
		if err := runScan(log, client, proc, cfg.Filter.Sender); err != nil {
			log.Error("initial scan failed", "err", err)
			os.Exit(1)
		}
	}

	if !cfg.Runtime.Idle {
		return
	}

	runIdle(ctx, log, client, proc, cfg.Filter.Sender)
}

func runScan(log *slog.Logger, client *mail.Client, proc *processor.Processor, sender string) error {
	log.Info("starting initial scan")
	uids, err := client.SearchUIDs(0, sender)
	if err != nil {
		return err
	}
	log.Info("scan: messages found", "count", len(uids))
	msgs, err := client.FetchMessages(uids)
	if err != nil {
		return err
	}
	for _, msg := range msgs {
		if err := proc.Process(msg); err != nil {
			return err
		}
	}
	log.Info("initial scan complete")
	return nil
}

func runIdle(ctx context.Context, log *slog.Logger, client *mail.Client, proc *processor.Processor, sender string) {
	log.Info("starting IDLE mode")

	if err := client.StartIdle(); err != nil {
		log.Error("start idle", "err", err)
		os.Exit(1)
	}

	var lastMaxUID int
	backoff := idleBackoffBase
	consecutive := 0

	for {
		select {
		case <-ctx.Done():
			log.Info("shutting down")
			if err := client.StopIdle(); err != nil {
				log.Warn("stop idle on shutdown", "err", err)
			}
			return

		case <-client.IdleEvents():
			if err := handleIdleEvent(ctx, log, client, proc, sender, &lastMaxUID); err != nil {
				consecutive++
				if consecutive >= idleMaxRetries {
					log.Error("too many consecutive IDLE errors, giving up", "err", err)
					os.Exit(1)
				}
				log.Warn("IDLE event error, will retry", "attempt", consecutive, "backoff", backoff, "err", err)
				select {
				case <-ctx.Done():
					log.Info("shutting down during backoff")
					return
				case <-time.After(backoff):
					backoff = min(backoff*2, idleBackoffMax)
				}
				// Restart IDLE after backoff so the select can receive the next event.
				if err := client.StartIdle(); err != nil {
					log.Error("restart idle after backoff failed", "err", err)
					os.Exit(1)
				}
			} else {
				consecutive = 0
				backoff = idleBackoffBase
			}
		}
	}
}

// handleIdleEvent stops IDLE, fetches and processes any new messages, then
// restarts IDLE. It returns an error on any transient failure so the caller
// can apply backoff and retry.
func handleIdleEvent(ctx context.Context, log *slog.Logger, client *mail.Client, proc *processor.Processor, sender string, lastMaxUID *int) error {
	if err := client.StopIdle(); err != nil {
		log.Warn("stop idle before fetch", "err", err)
		// Non-fatal: proceed to search/fetch; go-imap may have reconnected.
	}

	uids, err := client.SearchUIDs(*lastMaxUID+1, sender)
	if err != nil {
		return err
	}

	if len(uids) > 0 {
		msgs, err := client.FetchMessages(uids)
		if err != nil {
			return err
		}
		for _, msg := range msgs {
			if err := proc.Process(msg); err != nil {
				return err
			}
			if msg.UID > *lastMaxUID {
				*lastMaxUID = msg.UID
			}
		}
	}

	_ = ctx // reserved for future per-message context propagation
	if err := client.StartIdle(); err != nil {
		return err
	}
	return nil
}

