package sftp

import (
	"encoding"
	"sort"
)

// --------------------------------------------------------------------
// Process with 2 branch select, listening to each channel.
// 0) start of loop

// Branch A
// 1) Wait for ids to come in and add them to id list.

// Branch B
// 1) Wait for a packet comes in.
// 2) Add it to the packet list.
// 3) The heads of each list are then compared and if they have the same ids
//    the packet is sent out and the entries removed.
// 4) Goto step 2 Until the lists are emptied or the ids don't match.
// 5) Goto step 0.
// --------------------------------------------------------------------

type packetSender interface {
	sendPacket(encoding.BinaryMarshaler) error
}

type packetManager struct {
	requests  chan requestPacket
	responses chan responsePacket
	fini      chan struct{}
	incoming  uint32s
	outgoing  responsePackets
	sender    packetSender // connection object
}

func newPktMgr(sender packetSender) packetManager {
	s := packetManager{
		requests:  make(chan requestPacket, sftpServerWorkerCount),
		responses: make(chan responsePacket, sftpServerWorkerCount),
		fini:      make(chan struct{}),
		incoming:  make([]uint32, 0, sftpServerWorkerCount),
		outgoing:  make([]responsePacket, 0, sftpServerWorkerCount),
		sender:    sender,
	}
	go s.worker()
	return s
}

// for sorting/ordering incoming/outgoing
type uint32s []uint32
type responsePackets []responsePacket

func (u uint32s) Len() int           { return len(u) }
func (u uint32s) Swap(i, j int)      { u[i], u[j] = u[j], u[i] }
func (u uint32s) Less(i, j int) bool { return u[i] < u[j] }

func (r responsePackets) Len() int           { return len(r) }
func (r responsePackets) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r responsePackets) Less(i, j int) bool { return r[i].id() < r[j].id() }

// register incoming packets to be handled
// send id of 0 for packets without id
func (s packetManager) incomingPacket(pkt requestPacket) {
	s.requests <- pkt // buffer == sftpServerWorkerCount
}

// register outgoing packets as being ready
func (s packetManager) readyPacket(pkt responsePacket) {
	s.responses <- pkt
}

// shut down packetManager worker
func (s packetManager) close() {
	close(s.fini)
}

// process packets
func (s *packetManager) worker() {
	for {
		select {
		case pkt := <-s.requests:
			debug("incoming id: %v", pkt.id())
			s.incoming = append(s.incoming, pkt.id())
			if len(s.incoming) > 1 {
				sort.Sort(s.incoming)
			}
		case pkt := <-s.responses:
			debug("outgoing pkt: %v", pkt.id())
			s.outgoing = append(s.outgoing, pkt)
			if len(s.outgoing) > 1 {
				sort.Sort(s.outgoing)
			}
		case <-s.fini:
			return
		}
		s.maybeSendPackets()
	}
}

// send as many packets as are ready
func (s *packetManager) maybeSendPackets() {
	for {
		if len(s.outgoing) == 0 || len(s.incoming) == 0 {
			debug("break! -- outgoing: %v; incoming: %v",
				len(s.outgoing), len(s.incoming))
			break
		}
		out := s.outgoing[0]
		in := s.incoming[0]
		// 		debug("incoming: %v", s.incoming)
		// 		debug("outgoing: %v", outfilter(s.outgoing))
		if in == out.id() {
			s.sender.sendPacket(out)
			// pop off heads
			copy(s.incoming, s.incoming[1:])            // shift left
			s.incoming = s.incoming[:len(s.incoming)-1] // remove last
			copy(s.outgoing, s.outgoing[1:])            // shift left
			s.outgoing = s.outgoing[:len(s.outgoing)-1] // remove last
		} else {
			break
		}
	}
}

func outfilter(o []responsePacket) []uint32 {
	res := make([]uint32, 0, len(o))
	for _, v := range o {
		res = append(res, v.id())
	}
	return res
}
