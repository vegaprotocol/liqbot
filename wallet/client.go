// TODO: move to a shared lib?
package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/golang/protobuf/jsonpb"

	walletpb "code.vegaprotocol.io/protos/vega/wallet/v1"

	"code.vegaprotocol.io/liqbot/types"
)

type Client struct {
	walletURL string
	client    *http.Client
	token     string
	name      string
	pass      string
	pubKey    string // TODO: setup wallet and set this
}

func NewClient(walletURL string) *Client {
	return &Client{
		walletURL: walletURL,
		client:    http.DefaultClient,
	}
}

func (c *Client) CreateWallet(ctx context.Context, name, passphrase string) error {
	postBody, _ := json.Marshal(struct {
		Wallet     string `json:"wallet"`
		Passphrase string `json:"passphrase"`
	}{
		Wallet:     name,
		Passphrase: passphrase,
	})

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/api/v1/wallets", c.walletURL),
		bytes.NewBuffer(postBody),
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create wallet at vegawallet API: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %s", err)
	}

	sb := string(body)
	if strings.Contains(sb, "error") {
		return fmt.Errorf("error response: %s", sb)
	}

	type walletResponse struct {
		Token string `json:"token"`
	}

	result := new(walletResponse)
	if err := json.Unmarshal([]byte(sb), result); err != nil {
		return fmt.Errorf("failed to parse response: %s", err)
	}

	c.token = result.Token
	c.name = name
	c.pass = passphrase

	return nil
}

type tokenResponse struct {
	Token string `json:"token"`
}

func (c *Client) LoginWallet(ctx context.Context, name, passphrase string) error {
	postBody, _ := json.Marshal(struct {
		Wallet     string `json:"wallet"`
		Passphrase string `json:"passphrase"`
	}{
		Wallet:     name,
		Passphrase: passphrase,
	})

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/api/v1/auth/token", c.walletURL),
		bytes.NewBuffer(postBody),
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to login with vegawallet API: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %s", err)
	}

	sb := string(body)
	if strings.Contains(sb, "error") {
		return errors.New(sb)
	}

	result := new(tokenResponse)

	if err := json.Unmarshal([]byte(sb), &result); err != nil {
		return fmt.Errorf("failed to parse response: %s", err)
	}

	c.token = result.Token
	c.name = name
	c.pass = passphrase

	return nil
}

type keyPairResponse struct {
	Key *types.Key `json:"key"`
}

func (c *Client) GenerateKeyPair(ctx context.Context, passphrase string, meta []types.Meta) (*types.Key, error) {
	postBody, _ := json.Marshal(struct {
		Passphrase string       `json:"passphrase"`
		Meta       []types.Meta `json:"meta"`
	}{
		Passphrase: passphrase,
		Meta:       meta,
	})

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/api/v1/keys", c.walletURL),
		bytes.NewBuffer(postBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Add("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %s", err)
	}

	sb := string(body)
	if strings.Contains(sb, "error") {
		return nil, fmt.Errorf("error response: %s", sb)
	}

	result := new(keyPairResponse)

	if err := json.Unmarshal([]byte(sb), result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %s", err)
	}

	return result.Key, nil
}

type listKeysResponse struct {
	Keys []types.Key `json:"keys"`
}

func (c *Client) ListPublicKeys(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/api/v1/keys", c.walletURL),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Add("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %s", err)
	}

	sb := string(body)
	if strings.Contains(sb, "error") {
		return nil, fmt.Errorf("error response: %s", sb)
	}

	result := new(listKeysResponse)

	if err := json.Unmarshal([]byte(sb), result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %s", err)
	}

	pubKeys := make([]string, len(result.Keys))

	for i := range result.Keys {
		pubKeys[i] = result.Keys[i].Pub
	}

	return pubKeys, nil
}

type SignTxRequest struct {
	PubKey    string `json:"pubKey"`
	Propagate bool   `json:"propagate"`
}

// TODO: make a wallet service that would run commands instead of tx requests
func (c *Client) SignTx(ctx context.Context, request *walletpb.SubmitTransactionRequest) error {
	if c.pubKey != "" {
		request.PubKey = c.pubKey
	}

	m := jsonpb.Marshaler{Indent: "    "}

	request.Propagate = true

	data, err := m.MarshalToString(request)
	if err != nil {
		return fmt.Errorf("couldn't marshal input data: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/api/v1/command/sync", c.walletURL),
		bytes.NewBuffer([]byte(data)),
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Add("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %s", err)
	}

	sb := string(body)
	if strings.Contains(sb, "error") {
		if sb == `{"error":"session not found"}` {
			if err := c.LoginWallet(ctx, c.name, c.pass); err != nil {
				return fmt.Errorf("failed to login with vegawallet API: %v", err)
			}
		}
		return fmt.Errorf("error response: %s", sb)
	}

	return nil
}
