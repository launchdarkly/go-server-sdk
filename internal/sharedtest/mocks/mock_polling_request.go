package mocks

import "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

// Requester is a mock used in polling_data_source_test.go, to satisfy the
// datasource.Requester interface (used by PollingProcessor).
// Its purpose is to allow the PollingProcessor to be tested without involving actual HTTP operations.
type Requester struct {
	RequestAllRespCh chan RequestAllResponse
	PollsCh          chan struct{}
	CloserCh         chan struct{}
}

// RequestAllResponse is used to inject custom responses into the Requester,
// which will subsequently return them to the object under test.
type RequestAllResponse struct {
	Data   []ldstoretypes.Collection
	Cached bool
	Err    error
}

// NewPollingRequester constructs a Requester.
func NewPollingRequester() *Requester {
	return &Requester{
		RequestAllRespCh: make(chan RequestAllResponse, 100),
		PollsCh:          make(chan struct{}, 100),
		CloserCh:         make(chan struct{}),
	}
}

// Close closes the Requester's CloserCh.
func (r *Requester) Close() {
	close(r.CloserCh)
}

// BaseURI exists to fulfil the datasource.Requester interface; here it returns an empty string.
func (r *Requester) BaseURI() string {
	return ""
}

// FilterKey exists to fulfil the datasource.Requester interface; here it returns an empty string.
func (r *Requester) FilterKey() string {
	return ""
}

// Request blocks until a mock request is available on the RequestAllRespCh, or until closing
// via Close().
func (r *Requester) Request() ([]ldstoretypes.Collection, bool, error) {
	select {
	case resp := <-r.RequestAllRespCh:
		r.PollsCh <- struct{}{}
		return resp.Data, resp.Cached, resp.Err
	case <-r.CloserCh:
		return nil, false, nil
	}
}
