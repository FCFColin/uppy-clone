package game

// enqueueOutbound queues a broadcast for async delivery. Caller must hold r.mu.
func (r *Room) enqueueOutbound(payload []byte, excludePlayerID string, critical, skipRedis bool) {
	r.initOutboundManager()
	r.outbound.Enqueue(payload, excludePlayerID, critical, skipRedis)
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

// Bridge methods for test backward compatibility.

func (r *Room) snapshotConnTargetsLocked(excludePlayerID string) []connTarget {
	r.initOutboundManager()
	return r.outbound.source.SnapshotTargets(excludePlayerID)
}

func (r *Room) startOutboundLoop() {
	r.initOutboundManager()
	r.outbound.startLoop()
}

func (r *Room) deliverOutbound(msg outboundMsg) {
	r.initOutboundManager()
	r.outbound.deliver(msg)
}

func (r *Room) deliverToTargets(targets []connTarget, msg outboundMsg) {
	r.initOutboundManager()
	r.outbound.deliverToTargets(targets, msg)
}
