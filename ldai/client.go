package ldai

type ServerSDK interface {
}

type Client struct {
	sdk ServerSDK
}

func New(sdk ServerSDK) *Client {
	return &Client{}
}
