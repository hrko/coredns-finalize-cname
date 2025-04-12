package finalize

import (
	"context"
	"fmt"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

const pluginName = "finalize_cname"

var log = clog.NewWithPlugin(pluginName)

// Rewrite is plugin to rewrite requests internally before being handled.
type Finalize struct {
	Next plugin.Handler

	upstream *upstream.Upstream
	maxDepth int
}

func New() *Finalize {
	s := &Finalize{
		upstream: upstream.New(),
		maxDepth: 0,
	}

	return s
}

type FinalizeLoopKey struct{}

// ServeDNS implements the plugin.Handler interface.
func (s *Finalize) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	nw := nonwriter.New(w)
	rcode, err := plugin.NextOrFailure(s.Name(), s.Next, ctx, nw, r)
	if err != nil {
		return rcode, err
	}

	r = nw.Msg
	if r == nil {
		return dns.RcodeServerFailure, fmt.Errorf("no answer received")
	}
	qtype := r.Question[0].Qtype
	if len(r.Answer) > 0 && r.Answer[0].Header().Rrtype == dns.TypeCNAME && qtype != dns.TypeCNAME {
		log.Debugf("Finalizing CNAME for request: %+v", r)

		requestCount.WithLabelValues(metrics.WithServer(ctx)).Inc()
		defer recordDuration(ctx, time.Now())

		state := request.Request{W: w, Req: r}
		// emulate hashset in go; https://emersion.fr/blog/2017/sets-in-go/
		lookupedNames := make(map[string]struct{})
		depth := 0
		rrCname := r.Answer[0]
		rrs := []dns.RR{
			rrCname,
		}
		success := true

	Redo:
		targetName := rrCname.(*dns.CNAME).Target
		log.Debugf("Trying to resolve CNAME [%+v] via upstream", targetName)

		if s.maxDepth > 0 && depth >= s.maxDepth {
			maxDepthReachedCount.WithLabelValues(metrics.WithServer(ctx)).Inc()

			log.Errorf("Max depth %d reached for resolving CNAME records", s.maxDepth)
		} else if _, ok := lookupedNames[targetName]; ok {
			circularReferenceCount.WithLabelValues(metrics.WithServer(ctx)).Inc()

			log.Errorf("Detected circular reference in CNAME chain. CNAME [%s] already processed", targetName)
		} else {
			lookupMsg, err := s.upstream.Lookup(ctx, state, targetName, state.QType())
			lookupRRs := lookupMsg.Answer
			if err != nil {
				upstreamErrorCount.WithLabelValues(metrics.WithServer(ctx)).Inc()
				success = false

				log.Errorf("Failed to lookup CNAME [%+v] from upstream: [%+v]", rrCname, err)
			} else {
				if len(lookupRRs) == 0 {
					danglingCNameCount.WithLabelValues(metrics.WithServer(ctx)).Inc()
					success = false

					log.Errorf("Received no answer from upstream: [%+v]", lookupMsg)
				} else {
					if lookupRRs[0].Header().Rrtype == dns.TypeCNAME {
						depth++
						lookupedNames[targetName] = struct{}{}
						rrs = append(rrs, rrCname)
						rrCname = lookupRRs[0]
						goto Redo
					} else {
						rrs = append(rrs, lookupRRs...)
					}
				}
			}
		}

		if success {
			r.Answer = rrs
		}
	} else {
		log.Debug("Request is a CNAME type question, or didn't contain any answer or no CNAME")
	}

	err = w.WriteMsg(r)
	if err != nil {
		return dns.RcodeServerFailure, err
	}

	return dns.RcodeSuccess, nil
}

// Name implements the Handler interface.
func (al *Finalize) Name() string { return pluginName }

func recordDuration(ctx context.Context, start time.Time) {
	requestDuration.WithLabelValues(metrics.WithServer(ctx)).
		Observe(time.Since(start).Seconds())
}
