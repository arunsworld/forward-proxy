package forwardproxy

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/things-go/go-socks5"
	"github.com/things-go/go-socks5/statute"
)

func NewStaticFQDNBlocker(opts ...StaticFQDNBlockerOpt) socks5.RuleSet {
	result := &StaticFQDNBlocker{}
	for _, o := range opts {
		o(result)
	}
	return result
}

type StaticFQDNBlockerOpt func(*StaticFQDNBlocker)

type StaticFQDNBlocker struct {
	// internal
	blockedFQDN                   []blockList
	acceptLogging, blockedLogging bool
	histLogger                    HistLogger
	allowIPOnlyTraffic            bool
}

type blockList struct {
	name        string
	blockedFQDN map[string]struct{}
}

func (cc *StaticFQDNBlocker) Allow(ctx context.Context, req *socks5.Request) (context.Context, bool) {
	switch req.Command {
	case statute.CommandConnect:
		if allow, reason := cc.allow(req.DestAddr.FQDN, req.DestAddr.String()); !allow {
			if cc.blockedLogging {
				log.Printf("[StaticFQDNBlocker] Blocked traffic by %s to %s", reason, req.DestAddr.FQDN)
			}
			if cc.histLogger != nil {
				cc.histLogger.LogBlocked(req.DestAddr.FQDN)
			}
			return ctx, false
		} else {
			if cc.acceptLogging {
				if req.DestAddr.FQDN == "" {
					log.Printf("[StaticFQDNBlocker] Allowed traffic to %s", req.DestAddr.String())
				} else {
					log.Printf("[StaticFQDNBlocker] Allowed traffic to %s", req.DestAddr.FQDN)
				}
			}
			if cc.histLogger != nil {
				if req.DestAddr.FQDN == "" {
					cc.histLogger.LogAccepted(req.DestAddr.String())
				} else {
					cc.histLogger.LogAccepted(req.DestAddr.FQDN)
				}
			}
			return ctx, true
		}
	case statute.CommandBind:
		log.Println("[MyProxy] CommandBind")
	case statute.CommandAssociate:
		log.Println("[MyProxy] CommandAssociate")
	}
	return ctx, true
}

func (cc *StaticFQDNBlocker) allow(fqdn, address string) (bool, string) {
	if fqdn == "" {
		if cc.allowIPOnlyTraffic {
			return true, ""
		}
		return false, fmt.Sprintf("Empty FQDN for address: %s", address)
	}
	fqdnSplits := strings.Split(fqdn, ".")
	if len(fqdnSplits) < 2 {
		return false, fmt.Sprintf("Invalid Domain (%s)", address)
	}
	lenfqdnSplits := len(fqdnSplits)
	domainName := fqdnSplits[lenfqdnSplits-2] + "." + fqdnSplits[lenfqdnSplits-1]
	for _, bl := range cc.blockedFQDN {
		if _, ok := bl.blockedFQDN[fqdn]; ok {
			return false, bl.name
		}
		if _, ok := bl.blockedFQDN[domainName]; ok {
			return false, bl.name
		}
	}
	return true, ""
}

func WithStaticFQDNBlockList(name string, bl []string) StaticFQDNBlockerOpt {
	return func(cc *StaticFQDNBlocker) {
		blockedFQDN := make(map[string]struct{})
		for _, v := range bl {
			blockedFQDN[v] = struct{}{}
		}
		cc.blockedFQDN = append(cc.blockedFQDN, blockList{
			name:        name,
			blockedFQDN: blockedFQDN,
		})
	}
}

func WithAcceptLogging() StaticFQDNBlockerOpt {
	return func(cc *StaticFQDNBlocker) {
		cc.acceptLogging = true
	}
}

func WithBlockedLogging() StaticFQDNBlockerOpt {
	return func(cc *StaticFQDNBlocker) {
		cc.blockedLogging = true
	}
}

func WithHistLogger(hl HistLogger) StaticFQDNBlockerOpt {
	return func(cc *StaticFQDNBlocker) {
		cc.histLogger = hl
	}
}

func WithIPOnlyTrafficAllowed() StaticFQDNBlockerOpt {
	return func(cc *StaticFQDNBlocker) {
		cc.allowIPOnlyTraffic = true
	}
}
