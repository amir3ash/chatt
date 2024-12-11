package ws

import (
	"reflect"
	"testing"
)

func TestDispatchClientEvents(t *testing.T) {
	dispatcher := roomDispatcher{}

	var receivedClientEv clientEvent
	var receivedMsgEv msgEvent

	dispatcher.SubscribeOnClientEvents(func(e clientEvent) {
		receivedClientEv = e
	})

	dispatcher.SubscribeOnMsgEvents(func(e msgEvent) {
		receivedMsgEv = e
	})

	dispatcher.dispatch(clientEvent{evType: clientConnected})

	if receivedClientEv.evType != clientConnected {
		t.Errorf("clientEvent listener not called, gotEvent: %v", receivedClientEv)
	}

	if !reflect.DeepEqual(receivedMsgEv, msgEvent{}) {
		t.Errorf("dispatcher should not call msgEventListeners, gotEvent: %v", receivedMsgEv)
	}
}

func TestDispatchMessageEvents(t *testing.T) {
	dispatcher := roomDispatcher{}

	var receivedClientEv clientEvent
	var receivedMsgEv msgEvent

	dispatcher.SubscribeOnClientEvents(func(e clientEvent) {
		receivedClientEv = e
	})

	dispatcher.SubscribeOnMsgEvents(func(e msgEvent) {
		receivedMsgEv = e
	})

	dispatcher.dispatch(msgEvent{evType: msgSent})

	if receivedMsgEv.evType != msgSent {
		t.Errorf("clientEvent listener not called, gotEvent: %v", receivedMsgEv)
	}

	if !reflect.DeepEqual(receivedClientEv, clientEvent{}) {
		t.Errorf("dispatcher should not call clientEventListeners, gotEvent: %v", receivedClientEv)
	}
}
