package sftp

import (
	"encoding"
	"sort"
	"sync"
)

// The goal of the packetManager is to keep the outgoing packets in the same
// order as the incoming. This is due to some sftp clients requiring this
// behavior (eg. winscp).

type packetSender interface {
	sendPacket(encoding.BinaryMarshaler) error
}

type packetManager struct {
	requests  chan RequestPacket
	responses chan ResponsePacket
	fini      chan struct{}
	incoming  RequestPacketIDs
	outgoing  ResponsePackets
	sender    packetSender // connection object
	working   *sync.WaitGroup
}

func newPktMgr(sender packetSender) *packetManager {
	s := &packetManager{
		requests:  make(chan RequestPacket, SftpServerWorkerCount),
		responses: make(chan ResponsePacket, SftpServerWorkerCount),
		fini:      make(chan struct{}),
		incoming:  make([]uint32, 0, SftpServerWorkerCount),
		outgoing:  make([]ResponsePacket, 0, SftpServerWorkerCount),
		sender:    sender,
		working:   &sync.WaitGroup{},
	}
	go s.controller()
	return s
}

type ResponsePackets []ResponsePacket

func (r ResponsePackets) Sort() {
	sort.Slice(r, func(i, j int) bool {
		return r[i].Id() < r[j].Id()
	})
}

type RequestPacketIDs []uint32

func (r RequestPacketIDs) Sort() {
	sort.Slice(r, func(i, j int) bool {
		return r[i] < r[j]
	})
}

// register incoming packets to be handled
// send id of 0 for packets without id
func (s *packetManager) incomingPacket(pkt RequestPacket) {
	s.working.Add(1)
	s.requests <- pkt // buffer == SftpServerWorkerCount
}

// register outgoing packets as being ready
func (s *packetManager) readyPacket(pkt ResponsePacket) {
	s.responses <- pkt
	s.working.Done()
}

// shut down packetManager controller
func (s *packetManager) close() {
	// pause until current packets are processed
	s.working.Wait()
	close(s.fini)
}

// Passed a worker function, returns a channel for incoming packets.
// The goal is to process packets in the order they are received as is
// requires by section 7 of the RFC, while maximizing throughput of file
// transfers.
func (s *packetManager) workerChan(runWorker func(requestChan)) requestChan {

	rwChan := make(chan RequestPacket, SftpServerWorkerCount)
	for i := 0; i < SftpServerWorkerCount; i++ {
		runWorker(rwChan)
	}

	cmdChan := make(chan RequestPacket)
	runWorker(cmdChan)

	pktChan := make(chan RequestPacket, SftpServerWorkerCount)
	go func() {
		// start with cmdChan
		curChan := cmdChan
		for pkt := range pktChan {
			// on file open packet, switch to rwChan
			switch pkt.(type) {
			case *SSHFxpOpenPacket:
				curChan = rwChan
			// on file close packet, switch back to cmdChan
			// after waiting for any reads/writes to finish
			case *SSHFxpClosePacket:
				// wait for rwChan to finish
				s.working.Wait()
				// stop using rwChan
				curChan = cmdChan
			}
			s.incomingPacket(pkt)
			curChan <- pkt
		}
		close(rwChan)
		close(cmdChan)
		s.close()
	}()

	return pktChan
}

// process packets
func (s *packetManager) controller() {
	for {
		select {
		case pkt := <-s.requests:
			debug("incoming id: %v", pkt.Id())
			s.incoming = append(s.incoming, pkt.Id())
			if len(s.incoming) > 1 {
				s.incoming.Sort()
			}
		case pkt := <-s.responses:
			debug("outgoing pkt: %v", pkt.Id())
			s.outgoing = append(s.outgoing, pkt)
			if len(s.outgoing) > 1 {
				s.outgoing.Sort()
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
		if in == out.Id() {
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

//func outfilter(o []ResponsePacket) []uint32 {
//	res := make([]uint32, 0, len(o))
//	for _, v := range o {
//		res = append(res, v.Id())
//	}
//	return res
//}
