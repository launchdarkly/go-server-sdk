package mocks

import "github.com/launchdarkly/go-server-sdk/v6/subsystems/ldstoretypes"

type Requester struct {
	RequestAllRespCh      chan RequestAllResponse
	requestResourceRespCh chan requestResourceResponse
	PollsCh               chan struct{}
	CloserCh              chan struct{}
}

type RequestAllResponse struct {
	Data   []ldstoretypes.Collection
	Cached bool
	Err    error
}

type requestResourceResponse struct {
	item ldstoretypes.ItemDescriptor
	err  error
}

func NewPollingRequester() *Requester {
	return &Requester{
		RequestAllRespCh: make(chan RequestAllResponse, 100),
		PollsCh:          make(chan struct{}, 100),
		CloserCh:         make(chan struct{}),
	}
}

func (r *Requester) Close() {
	close(r.CloserCh)
}
func (r *Requester) BaseURI() string {
	return ""
}

func (r *Requester) Filter() string {
	return ""
}
func (r *Requester) Request() ([]ldstoretypes.Collection, bool, error) {
	select {
	case resp := <-r.RequestAllRespCh:
		r.PollsCh <- struct{}{}
		return resp.Data, resp.Cached, resp.Err
	case <-r.CloserCh:
		return nil, false, nil
	}
}
