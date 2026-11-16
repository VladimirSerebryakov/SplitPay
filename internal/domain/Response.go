package domain

import "encoding/json"

type CachedResponse struct {
	RequestHash  string          `json:"request_hash"`
	ResponseCode int             `json:"response_code"`
	ResponseBody json.RawMessage `json:"response_body"`
}
