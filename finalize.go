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

	upstream  *upstream.Upstream
	maxLookup int
}

func New() *Finalize {
	s := &Finalize{
		upstream:  upstream.New(),
		maxLookup: 10,
	}

	return s
}

// ServeDNS implements the plugin.Handler interface.
func (s *Finalize) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	// create a dummy writer, which not actually writes a response to the client
	nw := nonwriter.New(w)
	// call the rest of the plugin chain and pass the dummy writer to them
	rcode, err := plugin.NextOrFailure(s.Name(), s.Next, ctx, nw, r)
	if err != nil {
		return rcode, err
	}

	response := nw.Msg
	if response == nil {
		return dns.RcodeServerFailure, fmt.Errorf("no answer received")
	}
	qtype := response.Question[0].Qtype

	// skip if the question type is CNAME
	if qtype == dns.TypeCNAME {
		log.Debug("Request is a CNAME type question, skipping")
		goto Out
	}

	if len(response.Answer) == 0 {
		log.Debug("No answer received, skipping")
		goto Out
	}

	// skip if the answer is already finalized
	for _, rr := range response.Answer {
		if rr.Header().Rrtype != dns.TypeCNAME {
			log.Debugf("Answer is already finalized: %+v, skipping", rr)
			goto Out
		}
	}

	// create a block scope since `goto` cannot jump over variable declarations
	{
		log.Debugf("Finalizing CNAME for request: %+v", response)

		state := request.Request{W: w, Req: response}

		requestCount.WithLabelValues(metrics.WithServer(ctx)).Inc()
		defer recordDuration(ctx, time.Now())

		// emulate hashset in go; https://emersion.fr/blog/2017/sets-in-go/
		lookupedNames := make(map[string]struct{})
		lookupCnt := 0
		// copy the answer to avoid modifying the original
		rrs := make([]dns.RR, 0, len(response.Answer))
		copy(rrs, response.Answer)

		targetName, err := findLastTarget(rrs, state.QName())
		if err != nil {
			log.Errorf("Failed to find last target in CNAME chain: %v", err)
			goto Out
		}

	Redo:
		log.Debugf("Trying to resolve CNAME [%+v] via upstream", targetName)

		if s.maxLookup > 0 && lookupCnt >= s.maxLookup {
			maxLookupReachedCount.WithLabelValues(metrics.WithServer(ctx)).Inc()
			log.Errorf("Max lookup %d reached for resolving CNAME records", s.maxLookup)
			goto Out
		}
		lookupCnt++

		if _, ok := lookupedNames[targetName]; ok {
			circularReferenceCount.WithLabelValues(metrics.WithServer(ctx)).Inc()
			log.Errorf("Detected circular reference in CNAME chain. CNAME [%s] already processed", targetName)
			goto Out
		}

		lookupMsg, err := s.upstream.Lookup(ctx, state, targetName, state.QType())
		if err != nil {
			upstreamErrorCount.WithLabelValues(metrics.WithServer(ctx)).Inc()
			log.Errorf("Failed to lookup CNAME [%+v] from upstream: [%+v]", targetName, err)
			goto Out
		}

		lookupRRs := lookupMsg.Answer
		if len(lookupRRs) == 0 {
			danglingCNameCount.WithLabelValues(metrics.WithServer(ctx)).Inc()
			log.Errorf("Received no answer from upstream: [%+v]", lookupMsg)
			goto Out
		}

		rrs = append(rrs, lookupRRs...)

		// check if answer is finalized
		for _, rr := range lookupRRs {
			if rr.Header().Rrtype != dns.TypeCNAME {
				log.Debugf("Recieved finalized answer: %+v", lookupRRs)
				response.Answer = rrs
				goto Out
			}
		}

		// add the CNAME to the list of processed names
		lookupedNames[targetName] = struct{}{}

		// get the next target name
		targetName, err = findLastTarget(lookupRRs, targetName)
		if err != nil {
			log.Errorf("Failed to find last target in CNAME chain: %v", err)
			goto Out
		}
		log.Debugf("Found next target name: %s", targetName)
		goto Redo
	}

Out:
	err = w.WriteMsg(response)
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

// findLastTarget finds the last target in a CNAME chain.
func findLastTarget(rrs []dns.RR, qname string) (string, error) {
	nameToTarget := make(map[string]string)
	for _, rr := range rrs {
		if rr.Header().Rrtype == dns.TypeCNAME {
			cname := rr.(*dns.CNAME)
			nameToTarget[rr.Header().Name] = cname.Target
		}
	}

	if len(nameToTarget) == 0 {
		return "", fmt.Errorf("no CNAME records found")
	}

	// find the last target by following the chain
	nextName := qname
	depth := 0
	for {
		target, ok := nameToTarget[nextName]
		if !ok {
			if depth == 0 {
				return "", fmt.Errorf("no CNAME records found for %s", qname)
			}
			return nextName, nil
		}
		nextName = target
		depth++
		if depth > len(nameToTarget) {
			return "", fmt.Errorf("circular reference found in CNAME chain")
		}
	}
}
