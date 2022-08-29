package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type callbackService struct {
	baseURL string
}

func (c *callbackService) close() error {
	req, _ := http.NewRequest("DELETE", c.baseURL, nil)
	_, err := http.DefaultClient.Do(req)
	return err
}

func (c *callbackService) post(path string, params interface{}, responseOut interface{}) error {
	url := c.baseURL + path
	var postBody io.Reader
	if params != nil {
		data, _ := json.Marshal(params)
		postBody = bytes.NewBuffer(data)
	}
	resp, err := http.DefaultClient.Post(url, "application/json", postBody)
	if err != nil {
		//e.logger.Printf("Callback to %s failed: %s", url, err)
		return err
	}
	var body []byte
	if resp.Body != nil {
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
	}
	if resp.StatusCode >= 300 {
		message := ""
		if body != nil {
			message = " (" + string(body) + ")"
		}
		return fmt.Errorf("callback returned HTTP status %d%s", resp.StatusCode, message)
	}
	if responseOut != nil {
		if body == nil {
			return errors.New("expected a response body but got none")
		}
		if err = json.Unmarshal(body, responseOut); err != nil {
			return err
		}
	}
	return nil
}
