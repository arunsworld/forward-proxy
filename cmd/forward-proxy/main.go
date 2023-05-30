package main

import (
	"fmt"
	"log"
	"os"

	forwardproxy "github.com/arunsworld/forward-proxy"
	"github.com/things-go/go-socks5"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v2"
)

func main() {
	var port int
	var acceptLogging bool
	var blockedLogging bool
	var discardErrLogging bool
	var blockFile string
	var histLoggerFile string
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
				Aliases:     []string{"h"},
				Destination: &histLoggerFile,
			},
		},
		Action: func(cCtx *cli.Context) error {
			var opts []socks5.Option
			if !discardErrLogging {
				opts = append(opts, socks5.WithLogger(socks5.NewLogger(log.New(os.Stdout, "socks5: ", log.LstdFlags))))
			}
			blocker, err := standardStaticFQDNBlocker(blockFile, acceptLogging, blockedLogging)
			if err != nil {
				return err
			}
			opts = append(opts, socks5.WithRule(blocker))

			// Create a SOCKS5 server
			server := socks5.NewServer(opts...)
			if err := server.ListenAndServe("tcp", fmt.Sprintf(":%d", port)); err != nil {
				return err
			}
			return nil
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

type blockConfig struct {
	BlockList map[string][]string
}

func standardStaticFQDNBlocker(blockFile string, acceptLogging, blockedLogging bool) (socks5.RuleSet, error) {
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
	return forwardproxy.NewStaticFQDNBlocker(opts...), nil
}
