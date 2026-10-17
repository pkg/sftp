package sync

import (
	"testing"
)

func TestWorkPoolCloseWhileInUse(t *testing.T) {
	for range 1000 {
		p := NewWorkPool[int](4)

		var wg WaitGroup
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()

				for {
					ch, ok := p.Get()
					if !ok {
						return
					}
					p.Put(ch)
				}
			}()
		}

		p.Close()
		wg.Wait()
	}
}
