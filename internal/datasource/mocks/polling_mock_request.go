package mocks

import "github.com/launchdarkly/go-server-sdk/v6/subsystems/ldstoretypes"

type requester struct {
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

func NewPollingRequester() *requester {
	return &requester{
		RequestAllRespCh: make(chan RequestAllResponse, 100),
		PollsCh:          make(chan struct{}, 100),
		CloserCh:         make(chan struct{}),
	}
}

func (r *requester) Close() {
	close(r.CloserCh)
}

func (r *requester) Filter() string {
	return ""
}

func (r *requester) BaseURI() string {
	return ""
}

func (r *requester) Request() ([]ldstoretypes.Collection, bool, error) {
	select {
	case resp := <-r.RequestAllRespCh:
		r.PollsCh <- struct{}{}
		return resp.Data, resp.Cached, resp.Err
	case <-r.CloserCh:
		return nil, false, nil
	}
}
