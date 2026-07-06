package game

// broadcastOpts controls outbound delivery behavior.
type broadcastOpts struct {
	excludePlayerID string
	critical        bool
	skipRedis       bool
}

// enqueueOutbound queues a broadcast for async delivery. Caller must hold r.mu.
func (r *Room) enqueueOutbound(payload []byte, opts broadcastOpts) {
	r.initOutboundManager()
	r.outbound.Enqueue(payload, opts.excludePlayerID, opts.critical, opts.skipRedis)
}

func (r *Room) initOutboundManager() {
	if r.outbound == nil {
		r.outbound = NewOutboundManager(r.lobbyCode, r.instanceID, &r.syncOutbound, r.broadcaster, r, r.logger, &r.asyncWg)
	}
}

// stopOutbound stops the outbound delivery loop.
func (r *Room) stopOutbound() {
	if r.outbound == nil {
		return
	}
	r.outbound.Stop()
}


