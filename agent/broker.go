package agent

// Broker implements a basic pub/sub broker using channels.
type Broker[T any] struct {
	stopCh    chan struct{}
	publishCh chan T
	subCh     chan chan T
	unsubCh   chan chan T
}

func NewBroker[T any]() *Broker[T] {
	return &Broker[T]{
		stopCh:    make(chan struct{}),
		publishCh: make(chan T, 1),
		subCh:     make(chan chan T, 1),
		unsubCh:   make(chan chan T, 1),
	}
}

func (b *Broker[T]) Start() {
	subscribers := make(map[chan T]struct{})
	for {
		select {
		case <-b.stopCh:
			for s := range subscribers {
				close(s)
			}
			return
		case msgChan := <-b.subCh:
			subscribers[msgChan] = struct{}{}
		case msgChan := <-b.unsubCh:
			delete(subscribers, msgChan)
		case msg := <-b.publishCh:
			for msgChan := range subscribers {
				// non-blocking select
				select {
				case msgChan <- msg:
					// nothing todo
				default:
					// pass
				}
			}
		}
	}
}

func (b *Broker[T]) Stop() {
	close(b.stopCh)
}

func (b *Broker[T]) Subscribe() chan T {
	msgChan := make(chan T, 1)
	b.subCh <- msgChan
	return msgChan
}

func (b *Broker[T]) Unsubscribe(msgChan chan T) {
	b.unsubCh <- msgChan
	close(msgChan)
}

func (b *Broker[T]) Publish(msg T) {
	b.publishCh <- msg
}
