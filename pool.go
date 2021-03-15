package sftp

type bufPool struct {
	ch   chan []byte
	blen int
}

func newBufPool(depth, bufLen int) *bufPool {
	return &bufPool{
		ch:   make(chan []byte, depth),
		blen: bufLen,
	}
}

func (p *bufPool) Get() []byte {
	select {
	case b := <-p.ch:
		return b
	default:
		return make([]byte, p.blen)
	}
}

func (p *bufPool) Put(b []byte) {
	select {
	case p.ch <- b:
	default:
	}
}
