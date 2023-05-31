package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	forwardproxy "github.com/arunsworld/forward-proxy"
	"github.com/arunsworld/nursery"
	"github.com/things-go/go-socks5"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

func main() {
	var port int
	var acceptLogging bool
	var blockedLogging bool
	var discardErrLogging bool
	var blockFile string
	var histLoggerFile string
	var allowiponly bool
	var adminDomainName string
	var dnsFile string
	app := &cli.App{
		Name: "forward-proxy",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:        "port",
				Value:       10800,
				Aliases:     []string{"p"},
				EnvVars:     []string{"FORWARD_PROXY_PORT"},
				Destination: &port,
			},
			&cli.BoolFlag{
				Name:        "acceptlogging",
				Value:       false,
				Aliases:     []string{"a"},
				Destination: &acceptLogging,
			},
			&cli.BoolFlag{
				Name:        "blockedlogging",
				Value:       false,
				Aliases:     []string{"b"},
				Destination: &blockedLogging,
			},
			&cli.BoolFlag{
				Name:        "discarderrlogging",
				Value:       false,
				Aliases:     []string{"de"},
				Destination: &discardErrLogging,
			},
			&cli.StringFlag{
				Name:        "blockfile",
				Value:       "fqdn-block.yml",
				Aliases:     []string{"f"},
				Destination: &blockFile,
			},
			&cli.StringFlag{
				Name:        "histlogger",
				Value:       "hist-logger.yml",
				Aliases:     []string{"l"},
				Destination: &histLoggerFile,
			},
			&cli.BoolFlag{
				Name:        "allowiponly",
				Value:       false,
				Aliases:     []string{"ip"},
				Destination: &allowiponly,
			},
			&cli.StringFlag{
				Name:        "admindomain",
				Value:       "i",
				Destination: &adminDomainName,
			},
			&cli.StringFlag{
				Name:        "dns",
				Destination: &dnsFile,
			},
		},
		Action: func(cCtx *cli.Context) error {
			hlogger := newFileBasedHistLogger(histLoggerFile)

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			var opts []socks5.Option
			if !discardErrLogging {
				opts = append(opts, socks5.WithLogger(socks5.NewLogger(log.New(os.Stdout, "socks5: ", log.LstdFlags))))
			}
			blocker, err := standardStaticFQDNBlocker(blockFile, acceptLogging, blockedLogging, hlogger, allowiponly, adminDomainName)
			if err != nil {
				return err
			}
			opts = append(opts, socks5.WithRule(blocker))

			// experimental
			var dnsOverride map[string]net.IP
			if dnsFile != "" {
				v, err := dnsOverridesFromFile(dnsFile)
				if err != nil {
					return err
				}
				dnsOverride = v
			}
			opts = append(opts, socks5.WithResolver(newDNSResolver(adminDomainName, dnsOverride)))

			// Create a SOCKS5 server
			server := socks5.NewServer(opts...)
			l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
			if err != nil {
				return err
			}

			return nursery.RunConcurrently(
				func(_ context.Context, errCh chan error) {
					log.Printf("Serving on: %s", l.Addr().String())
					if err := server.Serve(l); err != nil {
						select {
						case <-ctx.Done():
							return
						default:
						}
						errCh <- err
					}
				},
				func(lctx context.Context, _ chan error) {
					select {
					case <-ctx.Done():
						log.Printf("Shutting down: %s", l.Addr().String())
						l.Close()
					case <-lctx.Done():
					}
				},
				func(lctx context.Context, _ chan error) {
					var loggerCloser io.Closer = hlogger
					select {
					case <-ctx.Done():
					case <-lctx.Done():
					}
					loggerCloser.Close()
				},
			)
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

type blockConfig struct {
	BlockList map[string][]string
}

func standardStaticFQDNBlocker(blockFile string, acceptLogging, blockedLogging bool, hl forwardproxy.HistLogger, allowiponly bool, adminDomainName string) (socks5.RuleSet, error) {
	contents, err := os.ReadFile(blockFile)
	if err != nil {
		return nil, err
	}
	input := blockConfig{}
	if err := yaml.Unmarshal(contents, &input); err != nil {
		return nil, err
	}
	opts := []forwardproxy.StaticFQDNBlockerOpt{}
	for name, bl := range input.BlockList {
		opts = append(opts, forwardproxy.WithStaticFQDNBlockList(name, bl))
	}
	if acceptLogging {
		opts = append(opts, forwardproxy.WithAcceptLogging())
	}
	if blockedLogging {
		opts = append(opts, forwardproxy.WithBlockedLogging())
	}
	if hl != nil {
		opts = append(opts, forwardproxy.WithHistLogger(hl))
	}
	if allowiponly {
		opts = append(opts, forwardproxy.WithIPOnlyTrafficAllowed())
	}
	if adminDomainName != "" {
		opts = append(opts, forwardproxy.WithAllowOverrideFQDN(map[string]struct{}{adminDomainName: {}}))
	}
	return forwardproxy.NewStaticFQDNBlocker(opts...), nil
}
