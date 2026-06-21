package bus

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	mbus "github.com/mustafaturan/bus/v3"
)

type EventBus struct {
	bus         *mbus.Bus
	idGenerator mbus.IDGenerator
	eventChan   chan event
	handleFuncs map[string]HandleFunc
	once        sync.Once
	ctx         context.Context
	async       bool
}

type event struct {
	evt       mbus.Event
	handleKey string
}

type idGenerator struct{}

type HandleFunc func(topic string, data interface{})

// use timestamp since  quantity of fern events won't be massive.
func (g *idGenerator) Generate() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func NewEventBus(ctx context.Context, async bool) *EventBus {
	idGenerator := &idGenerator{}
	bus, err := mbus.NewBus(idGenerator)
	if err != nil {
		log.Fatalf("init eventbus failed: %v", err)
	}

	bus.RegisterTopics(allTopics...)

	return &EventBus{
		bus:         bus,
		idGenerator: idGenerator,
		eventChan:   make(chan event),
		handleFuncs: make(map[string]HandleFunc),
		ctx:         ctx,
		async:       async,
	}
}

func (eb *EventBus) Subscribe(instance interface{}, name string, topicPattern string, handler HandleFunc) {
	// duplicated subscribe op may be undesired and we should avoid it.
	// So handler keys should be set carefully(with index or random id) for instances of the same component.
	key := fmt.Sprintf("%p:%s", instance, name)
	if _, exists := eb.handleFuncs[key]; exists {
		log.Panicf("handler with key %v is already subscribed", key)
	}

	h := mbus.Handler{
		Matcher: topicPattern,
		Handle: func(ctx context.Context, e mbus.Event) {
			if eb.async {
				eb.runLoop()
				eb.eventChan <- event{evt: e, handleKey: key}
			} else {
				eb.handleEvent(event{evt: e, handleKey: key})
			}
		},
	}
	eb.bus.RegisterHandler(key, h)
	eb.handleFuncs[key] = handler
	log.Printf("new subscriber found, name: %s, topics: %s", key, topicPattern)
}

func (eb *EventBus) Emit(topic string, data interface{}) {
	var source = ""
	pc, file, line, ok := runtime.Caller(1)
	if ok {
		f := runtime.FuncForPC(pc)
		shortFile := filepath.Base(file)
		source = fmt.Sprintf("%s:%d:%s", shortFile, line, f.Name())
	}

	// log.Println("event emitted from source: ", source)

	txID := eb.idGenerator.Generate()
	ctx := context.WithValue(eb.ctx, mbus.CtxKeyTxID, txID)
	ctx = context.WithValue(ctx, mbus.CtxKeySource, source)
	eb.bus.Emit(ctx, topic, data)
}

func (eb *EventBus) handleEvent(e event) {
	if handle := eb.handleFuncs[e.handleKey]; handle != nil {
		handle(e.evt.Topic, e.evt.Data)
	} else {
		log.Println("Warning: no handler registered for topic: ", e.evt.Topic)
	}
}

func (eb *EventBus) runLoop() {
	eb.once.Do(func() {
		go func() {
			for {
				select {
				case <-eb.ctx.Done():
					for e := range eb.eventChan {
						eb.handleEvent(e)
					}

					eb.close()
					log.Println("Eventbus quited")
					return
				case e := <-eb.eventChan:
					eb.handleEvent(e)
				}
			}
		}()
	})

}

// Since subscription may be dynamic, we add unsubscribe method(by instance) th remove the
// subscription.
func (eb *EventBus) Unsubscribe(instance interface{}) {
	keyPrefix := fmt.Sprintf("%p:", instance)

	for _, topic := range eb.bus.Topics() {
		for _, h := range eb.bus.TopicHandlerKeys(topic) {
			if strings.HasPrefix(h, keyPrefix) {
				eb.bus.DeregisterHandler(h)
			}
		}
	}

	for k := range eb.handleFuncs {
		if strings.HasPrefix(k, keyPrefix) {
			delete(eb.handleFuncs, k)
		}
	}
}

func (eb *EventBus) UnsubscribeByName(instance interface{}, name string) {
	key := fmt.Sprintf("%p:%s", instance, name)

	for _, topic := range eb.bus.Topics() {
		for _, h := range eb.bus.TopicHandlerKeys(topic) {
			if h == key {
				eb.bus.DeregisterHandler(h)
			}
		}
	}

	delete(eb.handleFuncs, key)
}

func (eb *EventBus) close() {
	for _, topic := range eb.bus.Topics() {
		for _, h := range eb.bus.TopicHandlerKeys(topic) {
			eb.bus.DeregisterHandler(h)
		}

		eb.bus.DeregisterTopics(topic)
	}

}
