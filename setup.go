package finalize

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

// init registers this plugin.
func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	finalize, err := parse(c)
	if err != nil {
		return plugin.Error(pluginName, err)
	}
	// Add the Plugin to CoreDNS, so Servers can use it in their plugin chain.
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		finalize.Next = next

		return finalize
	})

	log.Debug("Added plugin to server")

	return nil
}

func parse(c *caddy.Controller) (*Finalize, error) {
	finalizePlugin := New()
	for c.Next() {
		args := c.RemainingArgs()
		switch len(args) {
		case 0:
			// do nothing
		case 1:
			return nil, c.ArgErr()
		case 2:
			if strings.EqualFold("max_lookup", args[0]) {
				n, err := strconv.Atoi(args[1])
				if err != nil {
					return nil, err
				}
				if n <= 0 {
					return nil, fmt.Errorf("max_lookup parameter must be greater than 0")
				}
				finalizePlugin.maxLookup = n
			} else {
				return nil, fmt.Errorf("unsupported parameter %s for upstream setting", args[0])
			}
		default:
			return nil, c.ArgErr()
		}
	}

	log.Debug("Successfully parsed configuration")

	return finalizePlugin, nil
}
