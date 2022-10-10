package token

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"code.vegaprotocol.io/liqbot/types/num"
)

type FaucetService struct {
	faucetURL    string
	walletPubKey string
}

func NewFaucetService(faucetURL string, walletPubKey string) *FaucetService {
	return &FaucetService{
		faucetURL:    faucetURL,
		walletPubKey: walletPubKey,
	}
}

func (f FaucetService) Mint(ctx context.Context, assetID string, amount *num.Uint) error {
	postBody, _ := json.Marshal(struct {
		Party  string `json:"party"`
		Amount string `json:"amount"`
		Asset  string `json:"asset"`
	}{
		f.walletPubKey,
		amount.String(),
		assetID,
	})

	url := fmt.Sprintf("%s/api/v1/mint", f.faucetURL)
	reqBody := bytes.NewBuffer(postBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	sb := string(body)
	if strings.Contains(sb, "error") {
		return fmt.Errorf(sb)
	}

	return nil
}
