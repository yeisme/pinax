// Package inputintake exposes the optional HTTP transfer contract.
// Discover MCP action names from the owner capability resource before preparing.
package inputintake

import (
	"net/http"

	common "github.com/yeisme/runtime-plane/pkg/inputintake"
)

type File = common.File
type Request = common.View
type Receipt = common.Receipt
type Client = common.TransferClient

func NewClient(ownerBaseURL, transientGrantLink string, httpClient *http.Client) (*Client, error) {
	return common.NewTransferClient(ownerBaseURL, transientGrantLink, httpClient)
}
