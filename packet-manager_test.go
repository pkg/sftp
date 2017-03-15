package sftp

import (
	"encoding"
	"testing"

	"github.com/stretchr/testify/assert"
)

type _sender struct {
	sent chan encoding.BinaryMarshaler
}

func newsender() *_sender {
	return &_sender{make(chan encoding.BinaryMarshaler)}
}

func (s _sender) sendPacket(p encoding.BinaryMarshaler) error {
	s.sent <- p
	return nil
}

type fakepacket uint32

func (fakepacket) MarshalBinary() ([]byte, error) {
	return []byte{}, nil
}

func (fakepacket) UnmarshalBinary([]byte) error {
	return nil
}

func (f fakepacket) id() uint32 {
	return uint32(f)
}

type pair struct {
	in  fakepacket
	out fakepacket
}

var ttable1 = []pair{
	pair{fakepacket(0), fakepacket(0)},
	pair{fakepacket(1), fakepacket(1)},
	pair{fakepacket(2), fakepacket(2)},
	pair{fakepacket(3), fakepacket(3)},
}

var ttable2 = []pair{
	pair{fakepacket(0), fakepacket(0)},
	pair{fakepacket(1), fakepacket(4)},
	pair{fakepacket(2), fakepacket(1)},
	pair{fakepacket(3), fakepacket(3)},
	pair{fakepacket(4), fakepacket(2)},
}

var tables = [][]pair{ttable1, ttable2}

func TestPacketManager(t *testing.T) {
	sender := newsender()
	s := newPktMgr(sender)
	// 	go func() {
	// 		for _ = range s.workers {
	// 		}
	// 	}()
	for i := range tables {
		table := tables[i]
		for _, p := range table {
			s.incomingPacket(p.in)
		}
		for _, p := range table {
			s.readyPacket(p.out)
		}
		for _, p := range table {
			pkt := <-sender.sent
			id := pkt.(fakepacket).id()
			assert.Equal(t, id, p.in.id())
		}
	}
	s.close()
}
