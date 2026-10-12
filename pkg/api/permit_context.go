package api

import (
	"context"
	"sync"

	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/openapi"
)

// permitContextLoad is an API key scope request in flight for one config.
type permitContextLoad struct {
	done chan struct{}
	// err is the error the request returned to the caller that sent it, set before done is
	// closed. It is nil when the request succeeded, was ended by that caller's own context, or
	// panicked.
	err error
}

// permitContextLoads holds the scope requests in flight by config, so calls that share a config
// wait for its request instead of each sending one. It is package-level because PermitConfig has
// no field that could hold a guard without new exported API, and Build returns the config by
// value, so a guard in it would be copied; every API of a client shares the config pointer, so the
// pointer is the key. loadPermitContext checks and stores config.Context under this lock. API
// methods read it without the lock, only after loadPermitContext has seen it set.
var permitContextLoads = struct {
	sync.Mutex
	inFlight map[*config.PermitConfig]*permitContextLoad
}{inFlight: make(map[*config.PermitConfig]*permitContextLoad)}

// loadPermitContext sets a.config.Context from the API key scope when it is not set. One caller
// per config sends the scope request and the others wait for it: they share its error, or try
// again when the sender's own context ended it. The next call after a failed request sends a new
// one.
func (a *permitBaseApi) loadPermitContext(ctx context.Context) error {
	for {
		permitContextLoads.Lock()
		if a.config.Context != nil {
			permitContextLoads.Unlock()
			return nil
		}
		load, waiting := permitContextLoads.inFlight[a.config]
		if !waiting {
			load = &permitContextLoad{done: make(chan struct{})}
			permitContextLoads.inFlight[a.config] = load
		}
		permitContextLoads.Unlock()

		if !waiting {
			return a.requestPermitContext(ctx, load)
		}
		select {
		case <-load.done:
			if load.err != nil {
				return load.err
			}
		case <-ctx.Done():
			return errors.NewPermitContextErrorWithCause(
				"stopped waiting for the API key scope request", ctx.Err())
		}
	}
}

// requestPermitContext sends the API key scope request for load, stores the context it returns in
// a.config, and ends load.
func (a *permitBaseApi) requestPermitContext(
	ctx context.Context, load *permitContextLoad,
) (err error) {
	var permitContext *config.PermitContext
	// Deferred so that the entry is deleted and done closed even if the request panics; otherwise
	// the calls waiting for it, and every later call on this config, would block until their own
	// contexts end.
	defer func() {
		permitContextLoads.Lock()
		// Keep a context that SetContext stored while the request was in flight. SetContext
		// writes a.config.Context without taking this lock, and lazyLoadPermitContext and the
		// API methods read it without the lock, so a SetContext call that runs while other calls
		// on the same client run is a data race, here and wherever they read the context.
		// Callers must call SetContext before their other calls on that client, never at the
		// same time as them.
		if permitContext != nil && a.config.Context == nil {
			a.config.Context = permitContext
		}
		delete(permitContextLoads.inFlight, a.config)
		permitContextLoads.Unlock()
		if ctx.Err() == nil {
			load.err = err
		}
		close(load.done)
	}()
	// To ensure it's a regular client and not factsClient
	client := openapi.NewAPIClient(NewClientConfig(a.config))
	permitContext, err = config.PermitContextFactory(ctx, client, "", "", false)
	return err
}
